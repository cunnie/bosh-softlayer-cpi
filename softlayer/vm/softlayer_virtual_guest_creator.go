package vm

import (
	"fmt"
	"net"
	"time"

	bosherr "github.com/cloudfoundry/bosh-utils/errors"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"

	bslcommon "github.com/cloudfoundry/bosh-softlayer-cpi/softlayer/common"
	bslcstem "github.com/cloudfoundry/bosh-softlayer-cpi/softlayer/stemcell"
	datatypes "github.com/maximilien/softlayer-go/data_types"
	sl "github.com/maximilien/softlayer-go/softlayer"

	"github.com/cloudfoundry/bosh-softlayer-cpi/common"
)

const SOFTLAYER_VM_CREATOR_LOG_TAG = "SoftLayerVMCreator"

type softLayerVirtualGuestCreator struct {
	softLayerClient        sl.Client
	agentEnvServiceFactory AgentEnvServiceFactory

	agentOptions AgentOptions
	logger       boshlog.Logger
	vmFinder     Finder
}

func NewSoftLayerCreator(vmFinder Finder, softLayerClient sl.Client, agentOptions AgentOptions, logger boshlog.Logger) VMCreator {
	bslcommon.TIMEOUT = 120 * time.Minute
	bslcommon.POLLING_INTERVAL = 5 * time.Second

	return &softLayerVirtualGuestCreator{
		vmFinder:        vmFinder,
		softLayerClient: softLayerClient,
		agentOptions:    agentOptions,
		logger:          logger,
	}
}

func (c *softLayerVirtualGuestCreator) Create(agentID string, stemcell bslcstem.Stemcell, cloudProps VMCloudProperties, networks Networks, env Environment) (VM, error) {
	for _, network := range networks {
		switch network.Type {
		case "dynamic":
			if len(network.IP) == 0 {
				return c.createBySoftlayer(agentID, stemcell, cloudProps, networks, env)
			} else {
				return c.createByOSReload(agentID, stemcell, cloudProps, networks, env)
			}
		case "manual":
			return nil, bosherr.Error("Manual networking is not currently supported")
		case "vip":
			return nil, bosherr.Error("SoftLayer Not Support VIP netowrk")
		default:
			return nil, bosherr.Errorf("Softlayer Not Support This Kind Of Network: %s", network.Type)
		}
	}

	return nil, nil
}

// Private methods
func (c *softLayerVirtualGuestCreator) createBySoftlayer(agentID string, stemcell bslcstem.Stemcell, cloudProps VMCloudProperties, networks Networks, env Environment) (VM, error) {
	virtualGuestTemplate, err := CreateVirtualGuestTemplate(stemcell, cloudProps)
	if err != nil {
		return nil, bosherr.WrapError(err, "Creating VirtualGuest template")
	}

	virtualGuestService, err := c.softLayerClient.GetSoftLayer_Virtual_Guest_Service()
	if err != nil {
		return nil, bosherr.WrapError(err, "Creating VirtualGuestService from SoftLayer client")
	}

	virtualGuest, err := virtualGuestService.CreateObject(virtualGuestTemplate)
	if err != nil {
		return nil, bosherr.WrapError(err, "Creating VirtualGuest from SoftLayer client")
	}

	if cloudProps.EphemeralDiskSize == 0 {
		err = bslcommon.WaitForVirtualGuestLastCompleteTransaction(c.softLayerClient, virtualGuest.Id, "Service Setup")
		if err != nil {
			return nil, bosherr.WrapErrorf(err, "Waiting for VirtualGuest `%d` has Service Setup transaction complete", virtualGuest.Id)
		}
	} else {
		err = bslcommon.AttachEphemeralDiskToVirtualGuest(c.softLayerClient, virtualGuest.Id, cloudProps.EphemeralDiskSize, c.logger)
		if err != nil {
			return nil, bosherr.WrapError(err, fmt.Sprintf("Attaching ephemeral disk to VirtualGuest `%d`", virtualGuest.Id))
		}
	}

	vm, found, err := c.vmFinder.Find(virtualGuest.Id)
	if err != nil || !found {
		return nil, bosherr.WrapErrorf(err, "Cannot find VirtualGuest with id: %d.", virtualGuest.Id)
	}

	if len(cloudProps.BoshIp) == 0 {
		UpdateEtcHostsOfBoshInit(fmt.Sprintf("%s  %s", vm.GetPrimaryBackendIP(), vm.GetFullyQualifiedDomainName()))
		mbus, err := ParseMbusURL(c.agentOptions.Mbus, vm.GetPrimaryBackendIP())
		if err != nil {
			return nil, bosherr.WrapErrorf(err, "Cannot construct mbus url.")
		}
		c.agentOptions.Mbus = mbus
	} else {
		mbus, err := ParseMbusURL(c.agentOptions.Mbus, cloudProps.BoshIp)
		if err != nil {
			return nil, bosherr.WrapErrorf(err, "Cannot construct mbus url.")
		}
		c.agentOptions.Mbus = mbus

		switch c.agentOptions.Blobstore.Provider {
		case BlobstoreTypeDav:
			davConf := DavConfig(c.agentOptions.Blobstore.Options)
			UpdateDavConfig(&davConf, cloudProps.BoshIp)
		}
	}

	agentEnv := CreateAgentUserData(agentID, cloudProps, networks, env, c.agentOptions)

	err = vm.UpdateAgentEnv(agentEnv)
	if err != nil {
		return nil, bosherr.WrapError(err, "Updating VM's agent env")
	}

	if len(c.agentOptions.VcapPassword) > 0 {
		err = vm.SetVcapPassword(c.agentOptions.VcapPassword)
		if err != nil {
			return nil, bosherr.WrapError(err, "Updating VM's vcap password")
		}
	}

	return vm, nil
}

func (c *softLayerVirtualGuestCreator) createByOSReload(agentID string, stemcell bslcstem.Stemcell, cloudProps VMCloudProperties, networks Networks, env Environment) (VM, error) {
	virtualGuestService, err := c.softLayerClient.GetSoftLayer_Virtual_Guest_Service()
	if err != nil {
		return nil, bosherr.WrapError(err, "Creating VirtualGuestService from SoftLayer client")
	}

	var virtualGuest datatypes.SoftLayer_Virtual_Guest

	if common.IsPrivateSubnet(net.ParseIP(networks.First().IP)) {
		virtualGuest, err = virtualGuestService.GetObjectByPrimaryBackendIpAddress(networks.First().IP)
		c.logger.Info(SOFTLAYER_VM_CREATOR_LOG_TAG, fmt.Sprintf("OS reload on the server id %d with stemcell %d", virtualGuest.Id, stemcell.ID()))
	} else {
		virtualGuest, err = virtualGuestService.GetObjectByPrimaryIpAddress(networks.First().IP)
	}

	if err != nil || virtualGuest.Id == 0 {
		return nil, bosherr.WrapErrorf(err, "Could not find VirtualGuest by ip address: %s", networks.First().IP)
	}

	c.logger.Info(SOFTLAYER_VM_CREATOR_LOG_TAG, fmt.Sprintf("OS reload on VirtualGuest %d using stemcell %d", virtualGuest.Id, stemcell.ID()))

	vm, found, err := c.vmFinder.Find(virtualGuest.Id)
	if err != nil || !found {
		return nil, bosherr.WrapErrorf(err, "Cannot find virtualGuest with id: %d", virtualGuest.Id)
	}

	bslcommon.TIMEOUT = 4 * time.Hour
	err = vm.ReloadOS(stemcell)
	if err != nil {
		return nil, bosherr.WrapError(err, "Failed to reload OS")
	}

	if cloudProps.EphemeralDiskSize == 0 {
		err = bslcommon.WaitForVirtualGuestLastCompleteTransaction(c.softLayerClient, vm.ID(), "Service Setup")
		if err != nil {
			return nil, bosherr.WrapErrorf(err, "Waiting for VirtualGuest `%d` has Service Setup transaction complete", vm.ID())
		}
	} else {
		err = bslcommon.AttachEphemeralDiskToVirtualGuest(c.softLayerClient, vm.ID(), cloudProps.EphemeralDiskSize, c.logger)
		if err != nil {
			return nil, bosherr.WrapError(err, fmt.Sprintf("Attaching ephemeral disk to VirtualGuest `%d`", vm.ID()))
		}
	}

	if len(cloudProps.BoshIp) == 0 {
		UpdateEtcHostsOfBoshInit(fmt.Sprintf("%s  %s", vm.GetPrimaryBackendIP(), vm.GetFullyQualifiedDomainName()))
		mbus, err := ParseMbusURL(c.agentOptions.Mbus, vm.GetPrimaryBackendIP())
		if err != nil {
			return nil, bosherr.WrapErrorf(err, "Cannot construct mbus url.")
		}
		c.agentOptions.Mbus = mbus
	} else {
		mbus, err := ParseMbusURL(c.agentOptions.Mbus, cloudProps.BoshIp)
		if err != nil {
			return nil, bosherr.WrapErrorf(err, "Cannot construct mbus url.")
		}
		c.agentOptions.Mbus = mbus

		switch c.agentOptions.Blobstore.Provider {
		case BlobstoreTypeDav:
			davConf := DavConfig(c.agentOptions.Blobstore.Options)
			UpdateDavConfig(&davConf, cloudProps.BoshIp)
		}
	}

	vm, found, err = c.vmFinder.Find(virtualGuest.Id)
	if err != nil || !found {
		return nil, bosherr.WrapErrorf(err, "refresh VM with id: %d after os_reload", virtualGuest.Id)
	}

	agentEnv := CreateAgentUserData(agentID, cloudProps, networks, env, c.agentOptions)
	if err != nil {
		return nil, bosherr.WrapErrorf(err, "Cannot create agent env for virtual guest with id: %d", vm.ID())
	}

	err = vm.UpdateAgentEnv(agentEnv)
	if err != nil {
		return nil, bosherr.WrapError(err, "Updating VM's agent env")
	}

	if len(c.agentOptions.VcapPassword) > 0 {
		err = vm.SetVcapPassword(c.agentOptions.VcapPassword)
		if err != nil {
			return nil, bosherr.WrapError(err, "Updating VM's vcap password")
		}
	}
	return vm, nil
}
