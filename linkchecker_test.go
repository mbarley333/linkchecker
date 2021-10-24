package linkchecker_test

import (
	"fmt"
	"io"
	"linkchecker"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCheck(t *testing.T) {

	// test link crawl using ts with link to ts2
	ts2 := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		w.WriteHeader(http.StatusOK)

	}))

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<html><body><p><a href="%s"> a link</a></p></body></html>`, ts2.URL)

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
		{ResponseCode: 200, Url: ts2.URL},
	}

	got, err := l.Check(sites)
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(want, got) {
		t.Fatal(cmp.Diff(want, got))
	}

}

func TestNotFound(t *testing.T) {

	t.Parallel()

	ts2 := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		w.WriteHeader(http.StatusNotFound)

	}))

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<html><body><p><a href="%s"> a link</a></p></body></html>`, ts2.URL)

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
		{
			ResponseCode: 404,
			Url:          ts2.URL,
		},
	}

	got, err := l.Check(sites)
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(want, got) {
		t.Fatal(cmp.Diff(want, got))
	}

}

func TestCrawl(t *testing.T) {

	t.Parallel()

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, err := os.Open("testdata/htmltest.html")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		w.WriteHeader(http.StatusOK)
		io.Copy(w, f)

	}))

	l, err := linkchecker.NewLinkChecker()
	if err != nil {
		t.Fatal(err)
	}

	l.HTTPClient = ts.Client()

	url := ts.URL

	want := []linkchecker.Site{
		{URL: "http://127.0.0.1"},
	}

	got, err := l.Crawl(url)

	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(want, got) {
		t.Fatal(cmp.Diff(want, got))
	}

}

func TestParseBody(t *testing.T) {
	t.Parallel()

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, err := os.Open("testdata/htmltest.html")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		w.WriteHeader(http.StatusOK)
		io.Copy(w, f)

	}))

	url := ts.URL

	l, err := linkchecker.NewLinkChecker()
	if err != nil {
		t.Fatal(err)
	}

	l.HTTPClient = ts.Client()

	resp, err := l.HTTPClient.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	want := []linkchecker.Site{
		{URL: "http://127.0.0.1"},
	}
	got, err := linkchecker.ParseBody(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(want, got) {
		t.Fatal(cmp.Diff(want, got))
	}

}

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
