package linkchecker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/antchfx/htmlquery"
	"golang.org/x/time/rate"
)

type Result struct {
	ResponseCode  int
	Url           string
	Error         error
	ReferringSite string
}

type LinkChecker struct {
	HTTPClient  *http.Client
	Wg          sync.WaitGroup
	Results     chan Result
	output      io.Writer
	errorLog    io.Writer
	Scheme      string
	Domain      string
	Ratelimiter *rate.Limiter
	CheckLink   CheckLink
}

type Option func(*LinkChecker) error

func WithOutput(output io.Writer) Option {
	return func(l *LinkChecker) error {
		l.output = output
		return nil
	}
}

func WithErrorLog(errorLog io.Writer) Option {
	return func(l *LinkChecker) error {
		l.errorLog = errorLog
		return nil
	}
}

func WithConfigureRatelimiter(ratePerSec rate.Limit, burst int) Option {
	return func(l *LinkChecker) error {
		l.Ratelimiter = rate.NewLimiter(ratePerSec, burst)
		return nil
	}
}

func WithBufferedChannelSize(size int) Option {
	return func(l *LinkChecker) error {
		l.Results = make(chan Result, size)
		return nil
	}
}

func NewLinkChecker(opts ...Option) (*LinkChecker, error) {

	linkchecker := &LinkChecker{
		HTTPClient:  &http.Client{Timeout: 10 * time.Second},
		output:      os.Stdout,
		errorLog:    os.Stderr,
		Ratelimiter: rate.NewLimiter(12, 24),
		Results:     make(chan Result, 10),
		CheckLink: CheckLink{
			List: make(map[string]bool),
		},
	}

	for _, o := range opts {
		o(linkchecker)
	}

	return linkchecker, nil
}

func CheckSiteLinks(site string, opts ...Option) <-chan Result {
	l, err := NewLinkChecker()
	for _, o := range opts {
		o(l)
	}

	if err != nil {
		fmt.Fprintf(l.errorLog, "unable to create linkchecker struct, %s", err)
	}

	err = l.Check(site)
	if err != nil {
		fmt.Fprint(l.errorLog, err)
	}

	return l.Results

}

func (l *LinkChecker) Check(site string) error {

	url, err := url.Parse(site)
	if err != nil {
		return err
	}

	l.Scheme, l.Domain = url.Scheme, url.Host

	canonicalSite, err := l.CanonicaliseUrl(site)
	if err != nil {
		return err
	}

	referringSite := canonicalSite

	l.Crawl(canonicalSite, referringSite)

	close(l.Results)

	return nil

}

func (l *LinkChecker) Crawl(site string, referringSite string) {

	// get results for inital page
	l.Wg.Add(1)
	l.GetResult(site, referringSite)
	l.Wg.Wait()

	resp, err := l.GetResponse(site)
	if err != nil {
		fmt.Fprint(l.errorLog, err)
	}

	u, err := url.Parse(site)
	if err != nil {
		fmt.Fprint(l.errorLog, err)
	}

	// crawl pages and check links within domain
	// and only those not in the CheckLink.List map
	if u.Host == l.Domain && !l.IsCrawled(site) {

		// add site to CheckLink.List map to handle
		// link back (e.g pageA links pageB which links to pageA)
		// add site in if statement to allow the top level page to
		// be crawled.
		l.AddSite(site)

		// generate of list of links on page
		links, err := l.ParseBody(resp.Body)
		if err != nil {
			fmt.Fprintf(l.errorLog, "unable to generate site list, %s", err)
		}

		for _, link := range links {

			if !l.IsCrawled(link) {
				l.Wg.Add(1)
				go l.GetResult(link, site)
				l.Wg.Wait()
			}

		}

	}

}

func (l *LinkChecker) GetResult(site string, referringSite string) {

	defer l.Wg.Done()
	result := Result{
		Url:           site,
		ReferringSite: referringSite,
	}

	switch IsHttpTypeLink(site) {
	case true:
		resp, err := l.GetResponse(site)
		if err != nil {
			result.Error = err
		}
		result.ResponseCode = resp.StatusCode

		_, err = url.Parse(site)
		if err != nil {
			result.Error = err
		}

	case false:
		result.Error = fmt.Errorf("unable to check non http/https links: %s", site)
	}
	l.Results <- result

}

func (l *LinkChecker) GetResponse(site string) (*http.Response, error) {

	ctx := context.Background()
	err := l.Ratelimiter.Wait(ctx)
	if err != nil {
		return nil, err
	}

	_, err = l.IsHeaderAvailable(site)
	if err != nil {
		return nil, err
	}

	resp, err := l.HTTPClient.Get(site)
	if err != nil {

		return resp, err
	}

	return resp, nil
}

func (l *LinkChecker) ParseBody(body io.Reader) ([]string, error) {

	sites := []string{}
	doc, err := htmlquery.Parse(body)
	if err != nil {
		return nil, fmt.Errorf("unable to parse body, check if a valid io.Reader is being sent, %s", err)
	}

	list := htmlquery.Find(doc, "//a/@href")

	for _, n := range list {
		url := htmlquery.InnerText(n)

		if IsHttpTypeLink(url) {
			url, err = l.CanonicaliseChildUrl(url)
			if err != nil {
				fmt.Fprintf(l.errorLog, "unable to canonicalise url: %s, %s", url, err)
			}
		}
		sites = append(sites, url)

	}

	return sites, nil
}

func IsHttpTypeLink(link string) bool {

	return !strings.HasPrefix(strings.ToLower(link), "mailto:") && !strings.HasPrefix(strings.ToLower(link), "ftp:")
}

func (l *LinkChecker) IsHeaderAvailable(site string) (bool, error) {

	resp, err := l.HTTPClient.Head(site)
	if err != nil {
		return false, err
	}

	defer resp.Body.Close()

	return true, nil
}

type CheckLink struct {
	Locker sync.Mutex
	List   map[string]bool
}

func (l *LinkChecker) IsCrawled(site string) bool {

	result := false
	if l.CheckLink.List[site] {
		result = true
	}
	return result
}

func (l *LinkChecker) AddSite(site string) {

	l.CheckLink.Locker.Lock()
	defer l.CheckLink.Locker.Unlock()
	l.CheckLink.List[site] = true
}

func (l *LinkChecker) CanonicaliseUrl(site string) (string, error) {

	isUrl := false
	newUrl := site
	var err error

	// if no initial scheme from Check, try https and http
	if l.Scheme == "" {

		schemes := []string{"https", "http"}

		for _, scheme := range schemes {
			str := []string{scheme, "://", site}
			newUrl = strings.Join(str, "")
			isUrl, err = l.IsHeaderAvailable(newUrl)
			if err != nil {
				fmt.Fprintf(l.errorLog, "unable to use https scheme for %s, %s", site, err)
			}

			if isUrl {
				l.Scheme = scheme
				l.Domain = site
				break
			}

		}

	}

	return newUrl, nil
}

func (l *LinkChecker) CanonicaliseChildUrl(site string) (string, error) {

	newUrl := site
	var err error

	url, err := url.Parse(site)

	if err != nil {
		return "", err
	}

	scheme := ""
	if url.Scheme == "" {
		scheme = l.Scheme + "://"
	}

	domain := ""
	if url.Host == "" {
		domain = l.Domain + "/"
		//slash = "/"
	}

	// handle ./ link back to main page
	if site == "./" {
		// set to empty string and just use domain name
		site = ""
		//slash = ""
	} else if strings.Index(site, "/") == 0 {
		site = RemoveLeadingSlash(site)
	}

	// //slash needed for intrasite links with w/o domain (e.g <a href="home">)
	// slash := ""
	// assemble url
	str := []string{scheme, domain, site}
	newUrl = strings.Join(str, "")

	return newUrl, nil
}

func RemoveLeadingSlash(site string) string {

	for strings.Index(site, "/") == 0 {
		site = strings.TrimPrefix(site, "/")
	}

	return site
}

// CLI
func RunCLI() {

	if len(os.Args) < 2 {
		help(os.Args[0])
		os.Exit(1)
	}

	site := os.Args[1]
	if site == "help" {
		fmt.Println(os.Args[0])
		help(os.Args[0])
		os.Exit(0)
	}

	l, err := NewLinkChecker()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	done := make(chan bool)

	go func() {
		for {
			r, more := <-l.Results
			if more {
				fmt.Println(r)
			} else {
				fmt.Println("received all results")
				done <- true
				return
			}
		}

	}()

	err = l.Check(site)
	if err != nil {
		fmt.Fprintln(l.errorLog, err)
	}

	<-done

}

func help(cliArg string) {

	arg := "./linkchecker"

	// bit of a hack to handle when calling from go run cmd/main.go
	switch {
	// go run cmd.main.go
	case strings.Contains(cliArg, "go-build"):
		arg = "go run cmd/main.go"
	// docker run
	case cliArg == "/bin/linkchecker":
		arg = "docker run mbarley333/linkchecker:[tag] https://somewebpage123.com"
	}

	fmt.Fprintf(os.Stderr, `
	Description:
	  linkchecker will crawl a site and return the status of each link on the site
	
	Usage:
	%s https://somewebpage123.com
	`, arg)
}
