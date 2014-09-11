package vm_test

import (
	"errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/maximilien/bosh-softlayer-cpi/softlayer/vm"

	boshlog "bosh/logger"

	fakebslcpi "github.com/maximilien/bosh-softlayer-cpi/softlayer/cpi/fakes"
	fakevm "github.com/maximilien/bosh-softlayer-cpi/softlayer/vm/fakes"
)

var _ = Describe("SoftLayerFinder", func() {
	var (
		softLayerClient        *fakebslcpi.FakeClient
		agentEnvServiceFactory *fakevm.FakeAgentEnvServiceFactory
		hostBindMounts         *fakevm.FakeHostBindMounts
		guestBindMounts        *fakevm.FakeGuestBindMounts
		logger                 boshlog.Logger
		finder                 SoftLayerFinder
	)

	BeforeEach(func() {
		softLayerClient = fakebslcpi.New()
		agentEnvServiceFactory = &fakevm.FakeAgentEnvServiceFactory{}
		hostBindMounts = &fakevm.FakeHostBindMounts{}
		guestBindMounts = &fakevm.FakeGuestBindMounts{}
		logger = boshlog.NewLogger(boshlog.LevelNone)

		finder = NewSoftLayerFinder(
			softLayerClient,
			agentEnvServiceFactory,
			hostBindMounts,
			guestBindMounts,
			logger,
		)
	})

	Describe("Find", func() {
		It("returns VM and found as true if softlayer has container with VM ID as its handle", func() {
			agentEnvService := &fakevm.FakeAgentEnvService{}
			agentEnvServiceFactory.NewAgentEnvService = agentEnvService

			softLayerClient.Connection.ListReturns([]string{"non-matching-vm-id", "fake-vm-id"}, nil)

			expectedVM := NewSoftLayerVM(
				"fake-vm-id",
				softLayerClient,
				agentEnvService,
				hostBindMounts,
				guestBindMounts,
				logger,
			)

			vm, found, err := finder.Find("fake-vm-id")
			Expect(err).ToNot(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(vm).To(Equal(expectedVM))

			Expect(softLayerClient.Connection.ListCallCount()).To(Equal(1))
			Expect(softLayerClient.Connection.ListArgsForCall(0)).To(BeNil())
		})

		It("returns found as false if softlayer does not have container with VM ID as its handle", func() {
			softLayerClient.Connection.ListReturns([]string{"non-matching-vm-id"}, nil)

			vm, found, err := finder.Find("fake-vm-id")
			Expect(err).ToNot(HaveOccurred())
			Expect(found).To(BeFalse())
			Expect(vm).To(BeNil())
		})

		It("returns error if softlayer container listing fails", func() {
			softLayerClient.Connection.ListReturns(nil, errors.New("fake-list-err"))

			vm, found, err := finder.Find("fake-vm-id")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("fake-list-err"))
			Expect(found).To(BeFalse())
			Expect(vm).To(BeNil())
		})
	})
})