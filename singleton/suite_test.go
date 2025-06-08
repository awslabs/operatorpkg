package singleton_test

import (
	"testing"

	"github.com/awslabs/operatorpkg/singleton"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Singleton")
}

var _ = Describe("SingletonRateLimiter", func() {
	var rateLimiter *singleton.SingletonRateLimiter

	BeforeEach(func() {
		rateLimiter = singleton.NewSingletonRateLimiter("test-controller")
	})

	It("should return increasing delays on subsequent calls", func() {
		firstDelay := rateLimiter.Delay()
		Expect(firstDelay).To(BeNumerically(">=", 0))

		secondDelay := rateLimiter.Delay()
		Expect(secondDelay).To(BeNumerically(">=", firstDelay))

		thirdDelay := rateLimiter.Delay()
		Expect(thirdDelay).To(BeNumerically(">=", secondDelay))
	})
})
