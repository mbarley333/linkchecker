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
		Ratelimiter: rate.NewLimiter(20, 40),
		Results:     make(chan Result, 50),
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

	l.Wg.Add(1)
	go l.Crawl(canonicalSite, referringSite)
	l.Wg.Wait()

	close(l.Results)

	return nil

}

func (l *LinkChecker) Crawl(site string, referringSite string) {

	defer l.Wg.Done()

	if l.IsCrawled(site) {
		return
	}

	result := Result{
		Url:           site,
		ReferringSite: referringSite,
	}

	_, err := l.IsHeaderAvailable(site)
	if err != nil {
		result.Error = err
		l.Results <- result
		return
	}

	resp, err := l.HTTPClient.Get(site)
	if err != nil {
		result.Error = err
		l.Results <- result
		return
	}

	result.ResponseCode = resp.StatusCode

	u, err := url.Parse(site)
	if err != nil {
		result.Error = err
		l.Results <- result
		return
	}

	l.AddSite(site)

	if u.Host != l.Domain {
		l.Results <- result

		return
	}

	l.Results <- result

	// generate of list of links on page
	links, err := l.ParseBody(resp.Body)

	if err != nil {
		fmt.Fprintf(l.errorLog, "unable to generate site list, %s", err)
	}

	for _, link := range links {

		if !l.IsCrawled(link) {
			l.Wg.Add(1)

			ctx := context.Background()
			err := l.Ratelimiter.Wait(ctx)
			if err != nil {
				fmt.Fprintln(l.errorLog, err)
			}
			go l.Crawl(link, site)

		}

	}

}

func (l *LinkChecker) GetResponse(site string) (*http.Response, error) {

	_, err := l.IsHeaderAvailable(site)
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
	mutex sync.RWMutex
	List  map[string]bool
}

func (l *LinkChecker) IsCrawled(site string) bool {

	l.CheckLink.mutex.RLock()
	defer l.CheckLink.mutex.RUnlock()

	result := l.CheckLink.List[site]

	return result
}

func (l *LinkChecker) AddSite(site string) {

	l.CheckLink.mutex.Lock()
	defer l.CheckLink.mutex.Unlock()
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
				u, err := url.Parse(newUrl)
				if err != nil {
					fmt.Fprintf(l.errorLog, "unable to parse url %s, %s", newUrl, err)
				}
				l.Scheme = u.Scheme
				l.Domain = u.Host
				break
			}

		}

	}

	return newUrl, nil
}

func (l *LinkChecker) CanonicaliseChildUrl(site string) (string, error) {

	//newUrl := site
	canonical := site
	var err error

	if canonical == "./" {
		// set to empty string and just use domain name
		canonical = l.Scheme + "://" + l.Domain

	} else if strings.HasPrefix(site, "/") {
		canonical = RemoveLeadingSlash(canonical)
	}

	url, err := url.Parse(canonical)
	if err != nil {
		return "", err
	}

	if url.Host == "" {
		canonical = l.Domain + "/" + canonical
	}

	if url.Scheme == "" {
		canonical = l.Scheme + "://" + canonical
	}

	return canonical, nil
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

	// docker run
	if cliArg == "/bin/linkchecker" {
		arg = "docker run mbarley333/linkchecker:[tag]"
	}

	fmt.Fprintf(os.Stderr, `
	Description:
	  linkchecker will crawl a site and return the status of each link on the site
	
	Usage:
	%s https://somewebpage123.com
	`, arg)
}
