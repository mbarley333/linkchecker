package linkchecker_test

import (
	"fmt"
	"linkchecker"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// refactor
func TestCrawl(t *testing.T) {

	t.Parallel()

	fs := http.FileServer(http.Dir("./testdata"))

	ts := httptest.NewTLSServer(fs)

	l, err := linkchecker.NewLinkChecker()
	if err != nil {
		t.Fatal(err)
	}

	already := linkchecker.NewAlreadyCrawled()

	l.HTTPClient = ts.Client()

	url := ts.URL
	fmt.Println(url)

	want := []linkchecker.Result{
		{
			ResponseCode:  http.StatusOK,
			Url:           ts.URL,
			ReferringSite: ts.URL,
		},
	}

	l.Wg.Add(1)
	l.Crawl(url, url, l.Results, already)
	l.Wg.Wait()
	close(l.Results)

	r := []linkchecker.Result{}

	for result := range l.Results {
		r = append(r, result)
	}
	got := r

	fmt.Print(want, got)

	// if !cmp.Equal(want, got) {
	// 	t.Fatal(cmp.Diff(want, got))
	// }

}

func TestParseBody(t *testing.T) {
	t.Parallel()

	fs := http.FileServer(http.Dir("./testdata/about"))

	ts := httptest.NewTLSServer(fs)

	site := ts.URL

	l, err := linkchecker.NewLinkChecker()
	if err != nil {
		t.Fatal(err)
	}

	l.HTTPClient = ts.Client()

	url, err := url.Parse(site)
	if err != nil {
		t.Fatal(err)
	}

	l.Scheme, l.Domain = url.Scheme, url.Host

	resp, err := l.HTTPClient.Get(site)

	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	want := []string{
		ts.URL + "/home",
		ts.URL + "/zzz",
	}

	got, err := l.ParseBody(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(want, got) {
		t.Fatal(cmp.Diff(want, got))
	}

}

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

	want := []linkchecker.Result{
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
	}

	r := []linkchecker.Result{}

	for result := range l.Results {
		r = append(r, result)
	}

	got := r

	if !cmp.Equal(want, got) {
		t.Fatal(cmp.Diff(want, got))
	}

}

func TestIsHeaderAvailable(t *testing.T) {
	t.Parallel()

	fs := http.FileServer(http.Dir("./testdata"))

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

func TestHasSiteAlreadyBeenCrawled(t *testing.T) {
	t.Parallel()

	a := linkchecker.NewAlreadyCrawled()

	site := "https://example.org"

	want := false
	got := a.IsCrawled(site)

	if want != got {
		t.Fatalf("wanted: %v, got: %v", want, got)
	}

	a.AddSite(site)

	wantCrawled := true
	gotCrawled := a.IsCrawled(site)

	if wantCrawled != gotCrawled {
		t.Fatalf("wanted: %v, got: %v", wantCrawled, gotCrawled)
	}

}

func TestAddSiteToAlreadyCrawledList(t *testing.T) {
	t.Parallel()

	a := linkchecker.NewAlreadyCrawled()

	site := "https://example.com"

	result := a.IsCrawled(site)

	if !result {
		a.AddSite(site)
	}

	_, ok := a.List[site]

	want := true
	got := ok

	if want != got {
		t.Fatalf("wanted: %v, got: %v", want, got)
	}

}

func TestCanonicaliseUrl(t *testing.T) {

	t.Parallel()

	site := "./"

	want := "https://example.com"

	l, err := linkchecker.NewLinkChecker()
	if err != nil {
		t.Fatal(err)
	}

	l.Scheme = "https"
	l.Domain = "example.com"

	got, err := l.CanonicaliseUrl(site)
	if err != nil {
		t.Fatal(err)
	}

	if want != got {
		t.Fatalf("want: %s, got: %s", want, got)
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
