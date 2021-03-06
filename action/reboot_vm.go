package action

import (
	bosherr "github.com/cloudfoundry/bosh-utils/errors"

	bslcvm "github.com/cloudfoundry/bosh-softlayer-cpi/softlayer/vm"
)

type RebootVMAction struct {
	vmFinder bslcvm.Finder
}

func NewRebootVM(
	vmFinder bslcvm.Finder,
) (action RebootVMAction) {
	action.vmFinder = vmFinder
	return
}

func (a RebootVMAction) Run(vmCID VMCID) (interface{}, error) {
	vm, found, err := a.vmFinder.Find(int(vmCID))
	if err != nil {
		return nil, bosherr.WrapErrorf(err, "Finding vm '%s'", vmCID)
	}

	if found {
		err := vm.Reboot()
		if err != nil {
			return nil, bosherr.WrapErrorf(err, "Rebooting vm '%s'", vmCID)
		}
	}

	return nil, nil
}
