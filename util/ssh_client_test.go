package util_test

import (
	bscutil "github.com/maximilien/bosh-softlayer-cpi/util"
	bscutilfakes "github.com/maximilien/bosh-softlayer-cpi/util/fakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("SshClient", func() {
	var (
		sshClient bscutil.SshClient
	)

	Context("#ExecCommand", func() {
		var fakeSshClient *bscutilfakes.FakeSshClient

		BeforeEach(func() {
			fakeSshClient = &bscutilfakes.FakeSshClient{
				ExecCommandResult: "fake-result",
				ExecCommandError:  nil,
			}
			sshClient = fakeSshClient
		})

		It("executes the command using the SSH client", func() {
			output, err := sshClient.ExecCommand("fake-username", "fake-password", "localhost", "fake-command")
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeSshClient.Username).To(Equal("fake-username"))
			Expect(fakeSshClient.Password).To(Equal("fake-password"))
			Expect(fakeSshClient.Ip).To(Equal("localhost"))
			Expect(fakeSshClient.Command).To(Equal("fake-command"))

			Expect(output).To(Equal(fakeSshClient.ExecCommandResult))
		})
	})
})
