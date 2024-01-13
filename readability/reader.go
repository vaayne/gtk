package reader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
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
	sess        *session.Session
	browser     *rod.Browser
	cahceClient = cache.New(24*time.Hour, 7*24*time.Hour)
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

func Read(ctx context.Context, uri string, isFormatMarkdown bool, isUsingBrowser bool) (readability.Article, error) {
	parsedURL, _ := url.ParseRequestURI(uri)
	var article readability.Article
	var err error
	var html string

	if cahceClient != nil {
		article, ok := cahceClient.Get(uri)
		if ok {
			return article.(readability.Article), nil
		}
	}

	if isUsingBrowser {
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

	if isFormatMarkdown {
		converter := md.NewConverter("", true, nil)
		markdown, err := converter.ConvertString(article.Content)
		if err != nil {
			return article, fmt.Errorf("failed to convert HTML to Markdown, %v\n", err)
		}
		article.Content = markdown
	}
	article.Node = nil
	cahceClient.SetDefault(uri, article)
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
