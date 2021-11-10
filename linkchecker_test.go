package linkchecker_test

import (
	"fmt"
	"linkchecker"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCheck(t *testing.T) {
	t.Parallel()

	fs := http.FileServer(http.Dir("./testdata"))

	ts := httptest.NewTLSServer(fs)

	l, err := linkchecker.NewLinkChecker()
	if err != nil {
		t.Fatal(err)
	}
	l.HTTPClient = ts.Client()

	err = l.Check(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	wantResults := []linkchecker.Result{
		{
			ResponseCode:  http.StatusOK,
			Url:           ts.URL,
			ReferringSite: ts.URL,
		},
		{
			ResponseCode:  http.StatusOK,
			Url:           ts.URL + "/about",
			ReferringSite: ts.URL,
		},
		{
			ResponseCode:  http.StatusOK,
			Url:           ts.URL + "/home",
			ReferringSite: ts.URL,
		},
		{
			ResponseCode:  0,
			Url:           "mailto:home",
			ReferringSite: ts.URL,
			Error:         fmt.Errorf(`Head "mailto:home": unsupported protocol scheme "mailto"`),
		},
		{
			ResponseCode:  http.StatusNotFound,
			Url:           ts.URL + "/zzz",
			ReferringSite: ts.URL + "/about",
		},
	}

	// helper function to test for existence of error
	equateAnyError := cmp.Comparer(func(x, y error) bool {
		return (x == nil) == (y == nil)
	})

	gotResults := []linkchecker.Result{}

	for result := range l.Results {
		gotResults = append(gotResults, result)
	}

	wantResultsMap := make(map[string]linkchecker.Result)

	for _, r := range wantResults {
		wantResultsMap[r.Url] = r
	}

	// range over results and compare to map
	// to handle concurrent return of results
	for _, r := range gotResults {

		want := r
		got := wantResultsMap[r.Url]

		if !cmp.Equal(want, got, equateAnyError) {
			t.Fatal(cmp.Diff(want, got))
		}

	}

}

func TestParseBody(t *testing.T) {
	t.Parallel()

	l, err := linkchecker.NewLinkChecker()
	if err != nil {
		t.Fatal(err)
	}

	r := strings.NewReader(`<p>Here is <a href="https://example.com"> a link to a page</a></p>`)

	want := []string{"https://example.com"}

	got, err := l.ParseBody(r)
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(want, got) {
		t.Fatal(cmp.Diff(want, got))
	}

}

func TestIsHeaderAvailable(t *testing.T) {
	t.Parallel()

	fs := http.FileServer(http.Dir("testdata"))

	ts := httptest.NewTLSServer(fs)

	l, err := linkchecker.NewLinkChecker()
	if err != nil {
		t.Fatal(err)
	}
	l.HTTPClient = ts.Client()

	site := ts.URL + "/home.html"

	want := true
	got, err := l.IsHeaderAvailable(site)
	if err != nil {
		t.Fatal(err)
	}

	if want != got {
		t.Fatalf("wanted: %v, got: %v", want, got)
	}

}

func TestIsSiteInCheckLinkList(t *testing.T) {

	t.Parallel()

	l, err := linkchecker.NewLinkChecker()
	if err != nil {
		t.Fatal(err)
	}

	dummyUrl := "https://example.com"

	if l.IsCrawled(dummyUrl) {
		t.Fatalf("found url before it was added")
	}

	l.AddSite(dummyUrl)

	if !l.IsCrawled(dummyUrl) {
		t.Fatalf("could not find url after it was added")
	}

}

func TestCanonicaliseUrl(t *testing.T) {

	t.Parallel()

	ts := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))

	l, err := linkchecker.NewLinkChecker()
	if err != nil {
		t.Fatal(err)
	}
	l.HTTPClient = ts.Client()

	// check host only url
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	type testCase struct {
		url      string
		want     string
		scheme   string
		domain   string
		isParent bool
	}

	tcs := []testCase{
		{url: ts.URL, want: ts.URL, scheme: "https", domain: "", isParent: true},
		{url: u.Host, want: ts.URL, scheme: "", domain: "", isParent: true},
		{url: "./", want: ts.URL, scheme: "https", domain: u.Host, isParent: false},
		{url: "//about", want: ts.URL + "/about", scheme: "https", domain: u.Host, isParent: false},
		{url: "about", want: ts.URL + "/about", scheme: "https", domain: u.Host, isParent: false},
	}

	for _, tc := range tcs {

		l.Scheme = tc.scheme
		l.Domain = tc.domain
		got := ""
		if tc.isParent {
			got, err = l.CanonicaliseUrl(tc.url)
			if err != nil {
				t.Fatal(err)
			}
		} else if !tc.isParent {
			got, err = l.CanonicaliseChildUrl(tc.url)
			if err != nil {
				t.Fatal(err)
			}
		}

		if tc.want != got {
			t.Fatalf("want: %s, got: %s", tc.want, got)
		}

	}

}

func TestRemoveLeadingSlashes(t *testing.T) {
	t.Parallel()

	site := "///about"

	got := linkchecker.RemoveLeadingSlash(site)

	want := "about"

	if want != got {
		t.Fatalf("want: %s, got: %s", want, got)
	}

}
