package ondemand

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/koltyakov/gosip"
	"github.com/koltyakov/gosip/cpass"
)

var (
	cookieCache = map[string]*Cookies{}
	crypter     = cpass.Cpass("")
)

type AuthCnfg struct {
	SiteURL string `json:"siteUrl"` // SPSite or SPWeb URL, which is the context target for the API calls
}

// Gets the access token
func (c *AuthCnfg) GetAuth() (string, int64, error) {
	u, _ := url.Parse(c.SiteURL)

	// Check cached cookie per host
	cookies := cookieCache[u.Host]

	// Check disk cache
	if cookies == nil {
		cookies, _ = c.getCookieDiskCache()
	}

	if cookies != nil && !cookies.isExpired() {
		return cookies.toString(), cookies.getExpire(), nil
	}

	cookies, err := c.loadCookies(context.Background())
	if err != nil {
		return "", 0, err
	}

	_ = c.cacheCookieToDisk(cookies)

	cookieCache[u.Host] = cookies
	return cookies.toString(), cookies.getExpire(), nil
}

// Authenticates request
func (c *AuthCnfg) SetAuth(req *http.Request, httpClient *gosip.SPClient) error {
	authCookie, _, err := c.GetAuth()
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", authCookie)
	return nil
}

// Parses credentials from a provided JSON byte array content
func (c *AuthCnfg) ParseConfig(byteValue []byte) error {
	return json.Unmarshal(byteValue, &c)
}

// Reads private config with auth options
func (c *AuthCnfg) ReadConfig(privateFile string) error {
	f, err := os.Open(privateFile)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	byteValue, _ := io.ReadAll(f)
	return c.ParseConfig(byteValue)
}

// GetSiteURL gets SharePoint siteURL
func (c *AuthCnfg) GetSiteURL() string { return c.SiteURL }

// GetStrategy gets auth strategy name
func (c *AuthCnfg) GetStrategy() string { return "ondemand" }
