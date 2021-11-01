//+build integration

package linkchecker_test

import (
	"linkchecker"

	"github.com/google/go-cmp/cmp"
	"net/http"
	"testing"
)

// initiated by go test -tags=integration
func TestIntegrationCheck(t *testing.T) {

	site := "https://example.com"

	l, err := linkchecker.NewLinkChecker()
	if err != nil {
		t.Fatal(err)
	}

	want := []linkchecker.Result{
		{
			ResponseCode:  http.StatusOK,
			Url:           "https://example.com",
			ReferringSite: "https://example.com",
		},

		{
			ResponseCode:  http.StatusOK,
			Url:           "https://www.iana.org/domains/example",
			ReferringSite: "https://example.com",
		},
	}

	got, err := l.Check(site)
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(want, got) {
		t.Fatal(cmp.Diff(want, got))
	}

}
