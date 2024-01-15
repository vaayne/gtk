// Package cleanweb provides functionality for parsing web content.
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

// defaultUserAgent is the user agent string used for HTTP requests.
const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/86.0.4240.198 Safari/537.36"

// Cache interface defines methods for getting and setting values with a default expiration time.
type Cache interface {
	// Get retrieves the value associated with the provided key.
	Get(key string) (interface{}, bool)
	// SetDefault inserts a value into the cache using the provided key, with a default expiration time.
	SetDefault(key string, value interface{})
}

// Parser is a struct that holds the session, browser, timeout, format, and cache client for parsing web content.
type Parser struct {
	sess             *session.Session // The current session
	browser          *rod.Browser     // The browser instance used for web scraping
	timeout          time.Duration    // The maximum time allowed for the parsing operation
	isFormatMarkdown bool             // Flag indicating if the output should be formatted as Markdown
	cacheClient      Cache            // The cache client used for storing and retrieving parsed content
}

// NewParser creates a new Parser with a default session, timeout, and cache client.
func NewParser() *Parser {
	return &Parser{
		sess:        session.New(session.WithClientHelloID(utls.HelloChrome_100_PSK)), // Create a new session with a Chrome User-Agent
		timeout:     60 * time.Second,                                                 // Set a default timeout of 60 seconds
		cacheClient: cache.New(24*time.Hour, 7*24*time.Hour),                          // Initialize a new cache client with a default expiration time of 24 hours and a cleanup interval of 7 days
	}
}

// WithSession sets the session for the Parser and returns the Parser.
func (p *Parser) WithSession(sess *session.Session) *Parser {
	p.sess = sess // Set the session
	return p      // Return the Parser for method chaining
}

// WithBrowser sets the browser for the Parser and returns the Parser.
func (p *Parser) WithBrowser(browser *rod.Browser) *Parser {
	p.browser = browser // Set the browser
	return p            // Return the Parser for method chaining
}

// WithBrowserControlURL sets the browser for the Parser using a control URL and returns the Parser.
func (p *Parser) WithBrowserControlURL(browserURL string) *Parser {
	p.browser = rod.New().ControlURL(browserURL).MustConnect() // Connect to the browser using the control URL
	return p                                                   // Return the Parser for method chaining
}

// WithTimeout sets the timeout for the Parser and returns the Parser.
func (p *Parser) WithTimeout(timeout time.Duration) *Parser {
	p.timeout = timeout // Set the timeout
	return p            // Return the Parser for method chaining
}

// WithFormatMarkdown sets the format for the Parser to Markdown and returns the Parser.
func (p *Parser) WithFormatMarkdown() *Parser {
	p.isFormatMarkdown = true // Set the format to Markdown
	return p                  // Return the Parser for method chaining
}

func getCachekey(uri string, isFormatMarkdown bool) string {
	return fmt.Sprintf("cleanweb:%s:%v", uri, isFormatMarkdown)
}

// Parse is a method of the Parser struct that takes in a context and a URI string.
// It parses the content at the given URL and returns a readability.Article and an error.
func (p *Parser) Parse(ctx context.Context, uri string) (readability.Article, error) {
	// Initialize variables
	var article readability.Article
	var err error
	var html string

	// Parse the URI
	parsedURL, err := url.ParseRequestURI(uri)
	// If there's an error parsing the URI, return the error
	if err != nil {
		return article, fmt.Errorf("failed to parse url: %w", err)
	}

	// If the cache client is initialized
	if p.cacheClient != nil {
		// Try to get the article from the cache using the URI as the key
		article, ok := p.cacheClient.Get(getCachekey(uri, p.isFormatMarkdown))
		// If the article is in the cache, return the article and nil as the error
		if ok {
			return article.(readability.Article), nil
		}
	}

	// If the browser is initialized
	if p.browser != nil {
		// Read the content at the URI using the browser
		html, err = p.readWithBrowser(uri)
	} else {
		// Read the content at the URI using a GET request
		html, err = p.read(parsedURL)
	}
	// If there's an error reading the content, return the error
	if err != nil {
		return article, fmt.Errorf("failed to read url: %w", err)
	}

	// Parse the HTML content and return the article and any error
	return p.ParseHtml(ctx, html, uri)
}

// ParseHtml is a method of the Parser struct that takes in a context, an HTML string, and a URI string.
// It parses the HTML content and returns a readability.Article and an error.
func (p *Parser) ParseHtml(ctx context.Context, html string, uri string) (readability.Article, error) {
	// Parse the URI
	parsedURL, err := url.ParseRequestURI(uri)
	// If there's an error parsing the URI, set parsedURL to nil
	if err != nil {
		parsedURL = nil
	}
	// Use the readability package's FromReader function to parse the HTML content
	article, err := readability.FromReader(strings.NewReader(html), parsedURL)
	// If there's an error parsing the HTML content, return the error
	if err != nil {
		return article, fmt.Errorf("failed to parse %s, %v\n", uri, err)
	}

	// If the Parser is set to format as Markdown
	if p.isFormatMarkdown {
		// Create a new Converter
		converter := md.NewConverter("", true, nil)
		// Convert the article content to Markdown
		markdown, err := converter.ConvertString(article.Content)
		// If there's an error converting the content to Markdown, return the error
		if err != nil {
			return article, fmt.Errorf("failed to convert HTML to Markdown, %v\n", err)
		}
		// Set the article content to the converted Markdown
		article.Content = markdown
	}
	// Set the article's Node to nil
	article.Node = nil
	// Add the article to the cache with the URI as the key
	p.cacheClient.SetDefault(getCachekey(uri, p.isFormatMarkdown), article)
	// Return the article and nil as the error
	return article, nil
}

// read is a method of the Parser struct that takes in a URL.
// It sends a GET request to the given URL and returns the response body as a string and an error.
func (p *Parser) read(u *url.URL) (string, error) {
	// Convert the URL to a string
	uri := u.String()

	// Set the timeout for the session client
	p.sess.Client.Timeout = p.timeout
	// Create a new GET request
	req, _ := http.NewRequest("GET", uri, nil)
	// Set the User-Agent header for the request
	req.Header.Set("User-Agent", defaultUserAgent)
	// Send the GET request
	resp, err := p.sess.Get(uri)
	// If there's an error sending the request, return the error
	if err != nil {
		return "", err
	}
	// Ensure the response body is closed after the function returns
	defer resp.Body.Close()
	// Read all content from the response body
	content, err := io.ReadAll(resp.Body)
	// If there's an error reading the content, return the error
	if err != nil {
		return "", err
	}
	// Return the content as a string and any error
	return string(content), err
}

// readWithBrowser is a method of the Parser struct that takes in a URI string.
// It reads the content at the given URL using a browser and returns the HTML content as a string and an error.
func (p *Parser) readWithBrowser(uri string) (string, error) {
	// If the browser is not initialized, return an error
	if p.browser == nil {
		return "", fmt.Errorf("browser is not initialized")
	}

	// Open a new page in the browser with the given URI
	page := p.browser.MustPage(uri).Timeout(p.timeout)
	// Ensure the page is closed after the function returns
	defer page.MustClose()

	// Wait for the page to load
	page.MustWaitLoad()
	// Return the HTML content of the page
	return page.HTML()
}
