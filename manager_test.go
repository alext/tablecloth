package upgradeable_http

import (
	"net/http"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestManager(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Manager")
}

var _ = Describe("Adding listeners", func() {
	var (
		setupCount = 0
	)

	BeforeEach(func() {
		theManager = &manager{}
		setupFunc = func() {
			theManager.listeners = make(map[string]*GracefulListener)
			setupCount += 1
		}
	})

	AfterEach(func() {
		theManager.closeListeners()
	})

	It("Should add the listener using the given ident", func() {
		go ListenAndServe("127.0.0.1:8081", http.NotFoundHandler(), "one")
		time.Sleep(10 * time.Millisecond)

		listener := theManager.listeners["one"]
		Expect(listener).To(BeAssignableToTypeOf(&GracefulListener{}))
		Expect(listener.Addr().String()).To(Equal("127.0.0.1:8081"))
	})

	It("Should use an ident of default if none given", func() {
		go ListenAndServe("127.0.0.1:8081", http.NotFoundHandler())
		time.Sleep(10 * time.Millisecond)

		listener := theManager.listeners["default"]
		Expect(listener).To(BeAssignableToTypeOf(&GracefulListener{}))
		Expect(listener.Addr().String()).To(Equal("127.0.0.1:8081"))
	})

	It("Should return an error if given duplicate idents", func() {
		go ListenAndServe("127.0.0.1:8081", http.NotFoundHandler(), "foo")
		time.Sleep(10 * time.Millisecond)
		err := ListenAndServe("127.0.0.1:8082", http.NotFoundHandler(), "foo")

		Expect(err).To(MatchError("duplicate ident"))
	})
})
