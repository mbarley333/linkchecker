package linkchecker_test

import (
	"linkchecker"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// func TestNotFound(t *testing.T) {

// 	t.Parallel()

// 	ts2 := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

// 		w.WriteHeader(http.StatusNotFound)

// 	}))

// 	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

// 		w.WriteHeader(http.StatusOK)
// 		fmt.Fprintf(w, `<html><body><p><a href="%s"> a link</a></p></body></html>`, ts2.URL)

// 	}))

// 	sites := ts.URL

// 	l, err := linkchecker.NewLinkChecker()
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	l.HTTPClient = ts.Client()

// 	want := []linkchecker.Result{
// 		{
// 			ResponseCode:  200,
// 			Url:           ts.URL,
// 			ReferringSite: ts.URL,
// 		},
// 		{
// 			ResponseCode:  404,
// 			Url:           ts2.URL,
// 			ReferringSite: ts.URL,
// 		},
// 	}

// 	got, err := l.Check(sites)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	if !cmp.Equal(want, got) {
// 		t.Fatal(cmp.Diff(want, got))
// 	}

// }

// func TestCrawl(t *testing.T) {

// 	t.Parallel()

// 	fs := http.FileServer(http.Dir("./testdata"))

// 	ts := httptest.NewTLSServer(fs)

// 	l, err := linkchecker.NewLinkChecker()
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	already := linkchecker.NewAlreadyCrawled()
// 	// create channel of Result
// 	results := make(chan linkchecker.Result)

// 	// create wg for go routine
// 	l.Wg.Add(1)

// 	// spin up goroutine for results channel
// 	// this is the receiver part of the channel
// 	go l.ReceiveResultChannel(results)

// 	l.HTTPClient = ts.Client()

// 	url := ts.URL

// 	want := []linkchecker.Result{
// 		{
// 			ResponseCode:  http.StatusOK,
// 			Url:           ts.URL,
// 			ReferringSite: ts.URL,
// 		},
// 	}

// 	l.Crawl(url, url, results, already)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	l.Wg.Wait()

// 	got := l.Results

// 	if !cmp.Equal(want, got) {
// 		t.Fatal(cmp.Diff(want, got))
// 	}

// }

// func TestParseBody(t *testing.T) {
// 	t.Parallel()

// 	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		f, err := os.Open("testdata/about.html")
// 		if err != nil {
// 			t.Fatal(err)
// 		}
// 		defer f.Close()
// 		w.WriteHeader(http.StatusOK)
// 		io.Copy(w, f)

// 	}))

// 	url := ts.URL

// 	l, err := linkchecker.NewLinkChecker()
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	l.HTTPClient = ts.Client()

// 	resp, err := l.HTTPClient.Get(url)

// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	defer resp.Body.Close()

// 	want := []string{
// 		"http://localhost",
// 		"http://localhost:9090",
// 	}
// 	got, err := l.ParseBody(resp.Body)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	if !cmp.Equal(want, got) {
// 		t.Fatal(cmp.Diff(want, got))
// 	}

// }

func TestReceiverChannel(t *testing.T) {
	t.Parallel()

	l, err := linkchecker.NewLinkChecker()
	if err != nil {
		t.Fatal(err)
	}

	// create channel of Result
	results := make(chan linkchecker.Result)

	// create wg for go routine
	l.Wg.Add(1)

	// spin up goroutine for results channel
	// this is the receiver part of the channel
	go l.ReceiveResultChannel(results)

	// create result to feed into channel
	result := linkchecker.Result{
		ResponseCode: http.StatusNotFound,
		Url:          "127.0.0.1",
	}

	results <- result

	l.Wg.Wait()

	want := []linkchecker.Result{
		{
			ResponseCode: http.StatusNotFound,
			Url:          "127.0.0.1",
		},
	}

	got := l.Results

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

	got, err := l.Check(ts.URL)
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
		{
			ResponseCode:  http.StatusOK,
			Url:           ts.URL + "/home",
			ReferringSite: ts.URL + "/about",
		},
	}

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
