package linkchecker_test

import (
	"io"
	"linkchecker"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestNotFound(t *testing.T) {

	t.Parallel()

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		w.WriteHeader(http.StatusNotFound)

	}))

	client, err := linkchecker.NewLinkChecker()
	if err != nil {
		t.Fatal(err)
	}

	client.Base = ts.URL
	client.HTTPClient = ts.Client()

	got, err := client.Get(client.Base)
	if err != nil {
		t.Fatal(err)
	}

	want := linkchecker.Result{
		ResponseCode: 404,
		Url:          client.Base,
	}

	if !cmp.Equal(want, got) {
		t.Fatal(cmp.Diff(want, got))
	}

}

func TestWorkingLink(t *testing.T) {
	t.Parallel()

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
	}))

	client, err := linkchecker.NewLinkChecker()
	if err != nil {
		t.Fatal(err)
	}

	client.Base = ts.URL
	client.HTTPClient = ts.Client()

	got, err := client.Get(client.Base)
	if err != nil {
		t.Fatal(err)
	}

	want := linkchecker.Result{
		ResponseCode: 200,
		Url:          client.Base,
	}

	if !cmp.Equal(want, got) {
		t.Fatal(cmp.Diff(want, got))
	}

}

func TestCheck(t *testing.T) {

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, err := os.Open("testdata/htmltest.html")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		w.WriteHeader(http.StatusOK)
		io.Copy(w, f)

	}))

	sites := []string{
		ts.URL,
	}

	l, err := linkchecker.NewLinkChecker()
	if err != nil {
		t.Fatal(err)
	}

	l.HTTPClient = ts.Client()

	want := []linkchecker.Result{
		{ResponseCode: 200, Url: ts.URL},
	}
	got, err := l.Check(sites)
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(want, got) {
		t.Fatal(cmp.Diff(want, got))
	}

}
