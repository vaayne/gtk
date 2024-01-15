package cleanweb

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

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/go-rod/rod"
	"github.com/go-shiori/go-readability"
	"github.com/patrickmn/go-cache"
	utls "github.com/refraction-networking/utls"
	"github.com/vaayne/gtk/session"
)

const (
	defaultUserAgent      = "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/86.0.4240.198 Safari/537.36"
	defaultSessionTimeout = 60 * time.Second
)

var (
	once        sync.Once
	sess        *session.Session
	browser     *rod.Browser
	cahceClient Cache
)

func initSession() {
	sess = session.New(session.WithClientHelloID(utls.HelloChrome_100_PSK))
	sess.Timeout = defaultSessionTimeout
}

func initBrowser() {
	browserURL := os.Getenv("BROWSER_CONTROL_URL")
	if browserURL == "" {
		browserURL = "ws://localhost:3000"
	}

	browser = rod.New().ControlURL(browserURL).MustConnect()
}

func initCache() {
	cahceClient = cache.New(24*time.Hour, 7*24*time.Hour)
}

type Cache interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{}, d time.Duration)
}

type Parser struct {
	sess             *session.Session
	browser          *rod.Browser
	timeout          time.Duration
	isFormatMarkdown bool
	cacheClient      Cache
}

func NewParser() *Parser {
	once.Do(initSession)
	once.Do(initCache)
	return &Parser{
		sess:        sess,
		timeout:     defaultSessionTimeout,
		cacheClient: cahceClient,
	}
}

func (p *Parser) WithBrowser() *Parser {
	once.Do(initBrowser)
	p.browser = browser
	return p
}

func (p *Parser) WithBrowserControlURL(browserURL string) *Parser {
	p.browser = rod.New().ControlURL(browserURL).MustConnect()
	return p
}

func (p *Parser) WithTimeout(timeout time.Duration) *Parser {
	p.timeout = timeout
	return p
}

func (p *Parser) WithFormatMarkdown() *Parser {
	p.isFormatMarkdown = true
	return p
}

func (p *Parser) Parse(ctx context.Context, uri string) (readability.Article, error) {
	var article readability.Article
	var err error
	var html string
	parsedURL, err := url.ParseRequestURI(uri)
	if err != nil {
		return article, fmt.Errorf("failed to parse url: %w", err)
	}
	if cahceClient != nil {
		article, ok := cahceClient.Get(uri)
		if ok {
			return article.(readability.Article), nil
		}
	}

	if p.browser != nil {
		html, err = readWithBrowser(uri)
	} else {
		html, err = read(parsedURL)
	}
	if err != nil {
		return article, fmt.Errorf("failed to read url: %w", err)
	}
	article, err = readability.FromReader(strings.NewReader(html), parsedURL)
	if err != nil {
		return article, fmt.Errorf("failed to parse %s, %v\n", uri, err)
	}

	if p.isFormatMarkdown {
		converter := md.NewConverter("", true, nil)
		markdown, err := converter.ConvertString(article.Content)
		if err != nil {
			return article, fmt.Errorf("failed to convert HTML to Markdown, %v\n", err)
		}
		article.Content = markdown
	}
	article.Node = nil
	cahceClient.Set(uri, article, 24*time.Hour)
	return article, nil
}

func read(u *url.URL) (string, error) {
	if sess == nil {
		initSession()
	}

	uri := u.String()

	req, _ := http.NewRequest("GET", uri, nil)
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := sess.Get(uri)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(content), err
}

func readWithBrowser(uri string) (string, error) {
	if browser == nil {
		initBrowser()
	}

	page := browser.MustPage(uri).Timeout(defaultSessionTimeout)
	defer page.MustClose()

	page.MustWaitLoad()
	return page.HTML()
}
