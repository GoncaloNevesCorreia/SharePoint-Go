package ondemand

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func (c *AuthCnfg) cacheCookieToDisk(cookies *Cookies) error {
	tmpDir := filepath.Join(os.TempDir(), "gosip")
	cookieCachePath := c.getCookieCachePath()

	cookieCache, err := json.Marshal(cookies)
	if err != nil {
		return err
	}
	cookieCacheE, _ := crypter.Encode(fmt.Sprintf("%s", cookieCache))
	cookieCache = []byte(cookieCacheE)

	_ = os.MkdirAll(tmpDir, os.ModePerm)
	if err := os.WriteFile(cookieCachePath, cookieCache, 0644); err != nil {
		return err
	}
	return nil
}

func (c *AuthCnfg) CleanCookieCache() error {
	cookieCachePath := c.getCookieCachePath()
	u, err := url.Parse(c.SiteURL)
	if err != nil {
		return err
	}

	delete(cookieCache, u.Host)
	if err := os.Remove(cookieCachePath); err != nil {
		return err
	}
	return nil
}

// Gets local file system file path with token cache
func (c *AuthCnfg) getCookieCachePath() string {
	tmpDir := filepath.Join(os.TempDir(), "gosip")
	u, _ := url.Parse(c.SiteURL)
	return filepath.Join(tmpDir, c.GetStrategy()+"_"+u.Host)
}

// Reads cookies from temporary cache file
func (c *AuthCnfg) getCookieDiskCache() (*Cookies, error) {
	cookieCachePath := c.getCookieCachePath()

	cookieCache, err := os.ReadFile(cookieCachePath)
	if err != nil {
		return nil, err
	}
	cookieCacheD, _ := crypter.Decode(fmt.Sprintf("%s", cookieCache))
	cookieCache = []byte(cookieCacheD)

	cookies := &Cookies{}

	if err := json.Unmarshal(cookieCache, &cookies); err != nil {
		return nil, err
	}
	return cookies, nil
}

func (c *AuthCnfg) loadCookies(parent context.Context) (*Cookies, error) {

	edge, edgeNotFoundError := getEdgePath()

	if edgeNotFoundError != nil {
		return nil, edgeNotFoundError
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(edge),
		chromedp.Flag("headless", true),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(parent, opts...)
	defer cancelAlloc()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	defer cancelTimeout()

	foundCookies := Cookies{}

	err := chromedp.Run(ctx,
		network.Enable(),

		// Open the page so cookies can be created by redirects, JS, Set-Cookie headers, etc.
		chromedp.Navigate(c.SiteURL),
		chromedp.WaitReady("body", chromedp.ByQuery),

		chromedp.ActionFunc(func(ctx context.Context) error {
			cookies, err := network.GetCookies().
				WithURLs([]string{c.SiteURL}).
				Do(ctx)

			if err != nil {
				return err
			}

			for _, c := range cookies {
				if slices.Contains(cookieNames, strings.ToLower(c.Name)) {
					foundCookies = append(foundCookies, Cookie{c})
				}
			}

			if foundCookies.isEmpty() {
				return fmt.Errorf("cookies %q not found for %s", cookieNames, c.SiteURL)
			}

			return nil
		}),
	)

	if err != nil {
		return nil, err
	}

	return &foundCookies, nil
}

func getEdgePath() (string, error) {

	paths := []string{
		"msedge",
		"msedge.exe",
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
	}

	for _, path := range paths {
		if found, err := exec.LookPath(path); err == nil {
			return found, nil
		}
	}

	return "", errors.New("Microsoft Edge executable not found")

}
