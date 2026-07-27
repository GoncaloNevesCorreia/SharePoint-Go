package ondemand

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const SharepointProcessAppID = "sharepoint-app-automation-v1"
const DebugPort = "9732"

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

	edge, edgeNotFoundError := getEdgePath()

	if edgeNotFoundError != nil {
		return nil, edgeNotFoundError
	}

	userDataDir, err := os.MkdirTemp("", "edge-profile-*")

	if err != nil {
		return nil, fmt.Errorf("criar perfil temporário: %w", err)
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(edge),
		chromedp.DisableGPU,
		chromedp.NoSandbox,
		chromedp.UserDataDir(userDataDir),
		chromedp.Flag("disable-features", "IsolateOrigins,site-per-process,Translate,BlinkGenPropertyTrees,msImplicitSignin,msEdgeFirstRun,msEdgeFirstRunImplicitSignin,RendererCodeIntegrity"),
		chromedp.Flag("auth-server-whitelist", "*.sharepoint.com"),
		chromedp.Flag("sharepoint-id", SharepointProcessAppID),
		chromedp.Flag("remote-debugging-port", DebugPort),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(c.Ctx, opts...)
	defer cancelAlloc()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancelTimeout := context.WithTimeout(ctx, 5*time.Minute)
	defer cancelTimeout()

	foundCookies := Cookies{}

	err = runBrowser(ctx, c.SiteURL, &foundCookies)

	defer func() {
		clearPendingProcess()
		os.RemoveAll(userDataDir)
	}()

	if err != nil {
		if strings.HasSuffix(strings.TrimSpace(err.Error()), "chrome failed to start:") {

			foundCookies = Cookies{}

			err = reconnect(c.Ctx, c.SiteURL, &foundCookies)

			if err != nil {
				return nil, fmt.Errorf("ERRO_SENTINELONE")
			}

			return &foundCookies, nil
		}

		return nil, err
	}

	return &foundCookies, nil
}

func reconnect(ctx context.Context, url string, foundCookies *Cookies) error {
	wsURL, wsErr := obterWebSocketURL(DebugPort)

	if wsErr != nil {

		return fmt.Errorf("Bloqueado pelo EDR e falha na reconexão: %v", wsErr)
	}

	remoteCtx, remoteCancel := chromedp.NewRemoteAllocator(ctx, wsURL)
	defer remoteCancel()

	remoteBrowserCtx, remoteBrowserCancel := chromedp.NewContext(remoteCtx)
	defer remoteBrowserCancel()

	return runBrowser(remoteBrowserCtx, url, foundCookies)
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

func clearPendingProcess() {
	psScript := fmt.Sprintf(`Get-CimInstance Win32_Process -Filter "Name='msedge.exe'" | Where-Object { $_.CommandLine -match "%s" } | Invoke-CimMethod -MethodName Terminate`, SharepointProcessAppID)

	cmd := exec.Command("powershell", "-WindowStyle", "Hidden", "-NoProfile", "-Command", psScript)

	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	if err := cmd.Run(); err != nil {
		log.Printf("failed to clear pending process: %v", err)
	}
}

type edgeVersion struct {
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

func obterWebSocketURL(porta string) (string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%s/json/version", porta)
	cliente := &http.Client{Timeout: 2 * time.Second}

	for range 3 {
		resp, err := cliente.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			corpo, _ := io.ReadAll(resp.Body)

			var versao edgeVersion
			if err := json.Unmarshal(corpo, &versao); err == nil && versao.WebSocketDebuggerURL != "" {
				return versao.WebSocketDebuggerURL, nil
			}
		}
		time.Sleep(2 * time.Second)
	}

	return "", fmt.Errorf("não foi possível obter o websocket após 3 tentativas")
}

func runBrowser(ctx context.Context, url string, foundCookies *Cookies) error {
	return chromedp.Run(ctx,
		network.Enable(),

		// Open the page so cookies can be created by redirects, JS, Set-Cookie headers, etc.
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),

		chromedp.ActionFunc(func(ctx context.Context) error {
			cookies, err := network.GetCookies().
				WithURLs([]string{url}).
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
				return fmt.Errorf("cookies %q not found for %s", cookieNames, url)
			}

			return nil
		}),
	)
}
