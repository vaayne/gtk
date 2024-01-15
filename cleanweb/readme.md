# CleanWeb

CleanWeb is a Go package that provides functionality for parsing web content. It uses a combination of HTTP requests and a headless browser to fetch and parse web content. The parsed content can be returned as HTML or converted to Markdown. The package also includes caching functionality to store and retrieve parsed content.

## Installation

To install the package, use the following command:

```bash
go get github.com/vaayne/gtk/cleanweb
```

## Usage
Here is a basic example of how to use the package:
```go
package main

import (
    "context"
    "fmt"
    "github.com/vaayne/gtk/cleanweb"
)

func main() {
    // Create a new Parser
    parser := cleanweb.NewParser()

    // Set the Parser's session, browser, timeout, and format
    // parser := cleanweb.NewParser().parser.WithSession(mySession).WithBrowser(myBrowser).WithTimeout(60 * time.Second).WithFormatMarkdown()

    // Parse a URL
    article, err := parser.Parse(context.Background(), "https://example.com")
    if err != nil {
        fmt.Println("Error:", err)
        return
    }

    // Print the article's title and content
    fmt.Println("Title:", article.Title)
    fmt.Println("Content:", article.Content)
}
```
In this example, mySession and myBrowser should be replaced with your own session and browser instances. The WithFormatMarkdown() method is optional and can be removed if you want the content to be returned as HTML.

