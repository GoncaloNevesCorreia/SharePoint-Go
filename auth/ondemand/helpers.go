package ondemand

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type DevToolsTarget struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	Title                string `json:"title"`
	Description          string `json:"description"`
	URL                  string `json:"url"`
	WebSocketDebuggerUrl string `json:"webSocketDebuggerUrl"`
}

const SharepointProcessAppID = "sharepoint-app-automation-v1"
const DebugPort = "9732"

const USER_REQUIRED_ERROR = "USER_INTERACTION_REQUIRED"
const USER_CLOSED_BROWSER = "USER_CLOSED_BROWSER"
const USER_LOGIN_TIMEOUT = "USER_LOGIN_TIMEOUT"

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

func (c *AuthCnfg) loadCookies() (*Cookies, error) {

	// Fluxo de login automático
	foundCookies, err := Login(c.SiteURL, true)

	if err != nil {

		if err.Error() == USER_REQUIRED_ERROR {
			runtime.EventsEmit(c.Ctx, "LOGIN_EVENT", USER_REQUIRED_ERROR)

			// Fluxo de login manual
			foundCookies, err = Login(c.SiteURL, false)

			if err != nil {
				if err.Error() == USER_LOGIN_TIMEOUT {
					runtime.EventsEmit(c.Ctx, "LOGIN_EVENT", USER_LOGIN_TIMEOUT)
				}

				return nil, err
			}

		} else {
			return nil, err
		}

	}

	return foundCookies, nil
}

func Login(URL string, headless bool) (*Cookies, error) {
	edge, edgeNotFoundError := getEdgePath()

	if edgeNotFoundError != nil {
		return nil, edgeNotFoundError
	}

	userDataDir := filepath.Join(os.TempDir(), "SharePoint_Edge_Login")

	cleanUp(userDataDir)

	args := append(defaultFlags,
		fmt.Sprintf("--remote-debugging-port=%s", DebugPort),
		fmt.Sprintf("--user-data-dir=%s", userDataDir),
		fmt.Sprintf("--sharepoint-id=%s", SharepointProcessAppID),
		"--app=data:text/html,",
		"--window-size=500,600",
		"--window-position=100,100",

		// DEBUG FLAGS:
		// "--enable-logging=stderr",
		// "--v=1",
		// "--log-level=0",
	)

	if headless {
		args = append(args,
			"--headless",
			"--hide-scrollbars",
			"--mute-audio",
		)
	}

	cmd := exec.Command(edge, args...)

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	defer func() {
		cleanUp(userDataDir)
	}()

	devToolsTarget, err := getEdgeWebSocketDebugger()

	if err != nil {
		return nil, err
	}

	allocCtx, cancel := chromedp.NewRemoteAllocator(context.Background(), devToolsTarget.WebSocketDebuggerUrl)
	defer cancel()

	pageCtx, cancelCtx := chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(devToolsTarget.ID)))
	defer cancelCtx()

	ctx, cancelTimeout := context.WithTimeout(pageCtx, 5*time.Minute)
	defer cancelTimeout()

	var currentURL string

	// AuthURL := "https://www.office.com/login?prompt=select_account&ru=/launch/sharepoint/"

	// _, err = chromedp.RunResponse(ctx,
	// 	chromedp.Navigate(AuthURL),
	// 	chromedp.WaitReady("body", chromedp.ByQuery),
	// 	chromedp.Location(&currentURL),
	// )

	_, err = chromedp.RunResponse(ctx,
		chromedp.Navigate(URL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Location(&currentURL),
	)

	if err != nil {
		return nil, standardizeError(err)
	}

	if !strings.HasPrefix(currentURL, URL) {
		if headless {
			return nil, fmt.Errorf(USER_REQUIRED_ERROR)
		}

		err = chromedp.Run(ctx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				ticker := time.NewTicker(500 * time.Millisecond)

				isMinimised := false

				defer ticker.Stop()

				for {
					select {
					case <-ctx.Done():
						return ctx.Err()

					case <-ticker.C:

						if err := chromedp.Location(&currentURL).Do(ctx); err != nil {
							return err
						}

						if strings.HasPrefix(currentURL, URL) {
							return nil
						}

						if !isMinimised &&
							!strings.HasPrefix(currentURL, "https://login.microsoftonline.com") &&
							!strings.HasPrefix(currentURL, "https://login.microsoft.com") &&
							!strings.HasPrefix(currentURL, "https://accounts.google.com/v3/signin") &&
							!strings.HasPrefix(currentURL, "https://appleid.apple.com/auth/authorize") {
							windowID, _, err := browser.GetWindowForTarget().Do(ctx)

							if err != nil {
								return err
							}

							// Apply the minimized state to the window bounds
							bounds := &browser.Bounds{
								WindowState: browser.WindowStateMinimized,
							}

							browser.SetWindowBounds(windowID, bounds).Do(ctx)

							isMinimised = true
						}
					}
				}
			}),
		)

		if err != nil {
			return nil, standardizeError(err)
		}
	}

	foundCookies := &Cookies{}

	err = chromedp.Run(ctx,
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			cookies, err := network.GetCookies().
				WithURLs([]string{URL}).
				Do(ctx)

			if err != nil {
				return err
			}

			for _, c := range cookies {
				if slices.Contains(cookieNames, strings.ToLower(c.Name)) {
					*foundCookies = append(*foundCookies, Cookie{c})
				}
			}

			if foundCookies.isEmpty() {
				return fmt.Errorf("cookies %q not found for %s", cookieNames, URL)
			}

			return nil
		}),
	)

	if err != nil {
		return nil, standardizeError(err)
	}

	return foundCookies, nil
}

func getEdgePath() (string, error) {

	paths := []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
		"msedge.exe",
		"msedge",
	}

	for _, path := range paths {
		if found, err := exec.LookPath(path); err == nil {
			return found, nil
		}
	}

	return "", errors.New("Microsoft Edge executable not found")

}

func getEdgeWebSocketDebugger() (*DevToolsTarget, error) {
	url := fmt.Sprintf("http://127.0.0.1:%s/json/list", DebugPort)
	client := &http.Client{Timeout: 10 * time.Second}

	for range 3 {
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()

			var targets []DevToolsTarget

			if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
				return nil, fmt.Errorf("Failed to decode JSON: %v", err)
			}

			for _, t := range targets {
				if t.Type == "page" {
					return &t, nil
				}
			}
		}

		time.Sleep(2 * time.Second)
	}

	return nil, fmt.Errorf("Não foi possível obter o websocket após 3 tentativas")
}

func cleanUp(userDataDir string) {
	clearPendingProcess()

	time.Sleep(200 * time.Millisecond)

	for range 5 {
		err := os.RemoveAll(userDataDir)

		if err == nil {
			return
		}

		time.Sleep(200 * time.Millisecond)
	}
}

func clearPendingProcess() {
	psScript := fmt.Sprintf(`Get-CimInstance Win32_Process -Filter "Name='msedge.exe'" | Where-Object { $_.CommandLine -match "%s" } | Invoke-CimMethod -MethodName Terminate`, SharepointProcessAppID)

	cmd := exec.Command("powershell", "-WindowStyle", "Hidden", "-NoProfile", "-Command", psScript)

	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	if err := cmd.Run(); err != nil {
		log.Printf("failed to clear pending process: %v", err)
	}
}

func standardizeError(err error) error {
	switch err.Error() {
	case "context deadline exceeded":
		return fmt.Errorf(USER_LOGIN_TIMEOUT)
	case "context canceled":
		return fmt.Errorf(USER_CLOSED_BROWSER)
	case "page load error net::ERR_ABORTED":
		return fmt.Errorf(USER_CLOSED_BROWSER)
	default:
		return err
	}
}

var defaultFlags = []string{
	"--no-first-run",
	"--no-default-browser-check",

	// DEFAULT CHROMEDP FLAGS
	"--enable-features=NetworkService,NetworkServiceInProcess",
	"--disable-features=site-per-process,Translate,BlinkGenPropertyTrees",
	"--force-color-profile=srgb",

	"--disable-background-networking",
	"--disable-background-timer-throttling",
	"--disable-backgrounding-occluded-windows",
	"--disable-breakpad",
	"--disable-client-side-phishing-detection",
	"--disable-default-apps",
	"--disable-dev-shm-usage",
	"--disable-extensions",
	"--disable-hang-monitor",
	"--disable-ipc-flooding-protection",
	"--disable-popup-blocking",
	"--disable-prompt-on-repost",
	"--disable-renderer-backgrounding",
	"--disable-sync",
	"--disable-fre",

	"--metrics-recording-only",
	"--safebrowsing-disable-auto-update",
	// "--enable-automation",

	"--password-store=basic",
	"--use-mock-keychain",

	"--disable-component-update",
	"--disable-infobars",
}
