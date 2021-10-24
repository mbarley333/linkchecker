//+build integration

package linkchecker_test

import (
	"linkchecker"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// initiated by go test -tags=integration
func TestIntegrationCheck(t *testing.T) {

	sites := []string{
		"https://bitfieldconsulting.com", //href w/o schema and domain
		//"https://espn.com", //Get "https://www.tsn.ca/cfl": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
	}

	l, err := linkchecker.NewLinkChecker()
	if err != nil {
		t.Fatal(err)
	}

	want := []linkchecker.Result{
		{ResponseCode: 200, Url: "https://bitfieldconsulting.com"},
	}

	got, err := l.Check(sites)
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(want, got) {
		t.Fatal(cmp.Diff(want, got))
	}

}
