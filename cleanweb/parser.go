package cleanweb

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/go-rod/rod"
	"github.com/go-shiori/go-readability"
	"github.com/patrickmn/go-cache"
	utls "github.com/refraction-networking/utls"
	"github.com/vaayne/gtk/session"
)

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/86.0.4240.198 Safari/537.36"

type Cache interface {
	Get(key string) (interface{}, bool)
	SetDefault(key string, value interface{})
}

type Parser struct {
	sess             *session.Session
	browser          *rod.Browser
	timeout          time.Duration
	isFormatMarkdown bool
	cacheClient      Cache
}

func NewParser() *Parser {
	return &Parser{
		sess:        session.New(session.WithClientHelloID(utls.HelloChrome_100_PSK)),
		timeout:     60 * time.Second,
		cacheClient: cache.New(24*time.Hour, 7*24*time.Hour),
	}
}

func (p *Parser) WithSession(sess *session.Session) *Parser {
	p.sess = sess
	return p
}

func (p *Parser) WithBrowser(browser *rod.Browser) *Parser {
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
	if p.cacheClient != nil {
		article, ok := p.cacheClient.Get(uri)
		if ok {
			return article.(readability.Article), nil
		}
	}

	if p.browser != nil {
		html, err = p.readWithBrowser(uri)
	} else {
		html, err = p.read(parsedURL)
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
	p.cacheClient.SetDefault(uri, article)
	return article, nil
}

func (p *Parser) read(u *url.URL) (string, error) {
	uri := u.String()

	p.sess.Client.Timeout = p.timeout
	req, _ := http.NewRequest("GET", uri, nil)
	req.Header.Set("User-Agent", defaultUserAgent)
	resp, err := p.sess.Get(uri)
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

func (p *Parser) readWithBrowser(uri string) (string, error) {
	if p.browser == nil {
		return "", fmt.Errorf("browser is not initialized")
	}

	page := p.browser.MustPage(uri).Timeout(p.timeout)
	defer page.MustClose()

	page.MustWaitLoad()
	return page.HTML()
}
