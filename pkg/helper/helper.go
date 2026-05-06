package helper

import (
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"get-link-tg-bot/models"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sulton0011/errs"
)

var (
	resourceIDRegex   = regexp.MustCompile(`_(\d+)(?:\.htm|$)`)
	numericTokenRegex = regexp.MustCompile(`\d+`)
	nextDataRegex     = regexp.MustCompile(`(?s)<script id="__NEXT_DATA__" type="application/json">(.*?)</script>`)
	optionIDRegex     = regexp.MustCompile(`(?i)"optionId"\s*:\s*"?(\d+)"?`)
	optionIDAltRegex  = regexp.MustCompile(`(?i)"option_id"\s*:\s*"?(\d+)"?`)
	filenameIDRegex   = regexp.MustCompile(`(?i)\b\d{5,}_(\d{1,4})_[^"'\\/\s]+\.(?:mov|mp4|webm|avi)\b`)
	anyURLRegex       = regexp.MustCompile(`https?:\/\/[^\s"'<>\\]+`)
)

var legacyVideoLocaleMap = map[string]string{
	"www": "ru",
	"fr":  "fr",
	"br":  "br",
	"ru":  "ru",
}

var (
	freepikAuthRefreshURL        = "https://www.magnific.com/"
	freepikSecureTokenRefreshURL = "https://securetoken.googleapis.com/v1/token"
	freepikAuthRefreshMu         sync.Mutex
	freepikAuthClient            = &http.Client{Timeout: 30 * time.Second}
	freepikSecureTokenClient     = &http.Client{Timeout: 30 * time.Second}
)

var supportedAssetDomains = []string{
	"magnific.com",
	"freepik.com",
}

type endpointCandidate struct {
	label string
	url   string
}

type assetPageData struct {
	Icon        *iconPageData
	Video       *videoPageData
	RegularType string
}

type iconPageData struct {
	ID         int              `json:"id"`
	FreeSVG    bool             `json:"freeSvg"`
	Thumbnails iconThumbnailSet `json:"thumbnails"`
}

type iconThumbnailSet struct {
	Small  mediaURL `json:"small"`
	Medium mediaURL `json:"medium"`
	Large  mediaURL `json:"large"`
}

type videoPageData struct {
	ID          int           `json:"id"`
	Premium     bool          `json:"premium"`
	Orientation string        `json:"orientation"`
	VideoSrc    string        `json:"videoSrc"`
	Previews    []mediaURL    `json:"previews"`
	Options     []videoOption `json:"options"`
	URLs        []string
	OptionIDs   []string
}

type mediaURL struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type videoOption struct {
	ID         int    `json:"id"`
	Active     bool   `json:"active"`
	IsOriginal bool   `json:"isOriginal"`
	Container  string `json:"container"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Size       int    `json:"size"`
}

type model3DMetadata struct {
	ID               int         `json:"id"`
	HasBlendFile     bool        `json:"hasBlendFile"`
	HasObjFile       bool        `json:"hasObjFile"`
	HasFbxFile       bool        `json:"hasFbxFile"`
	WalletID         interface{} `json:"walletId"`
	SearchExpression string      `json:"searchExpression"`
	Specifications   struct {
		IncludeTextures bool `json:"includeTextures"`
	} `json:"specifications"`
}

type FreepikAuthStatus struct {
	Source    string
	HasToken  bool
	ExpiresAt time.Time
}

func GetPathFreepik(link string) string {
	u, err := normalizeFreepikURL(link)
	if err != nil {
		return ""
	}

	path := strings.Trim(u.Path, "/")
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func IsSupportedAssetURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return isSupportedAssetHost(u.Hostname())
}

func GetLanguageFreepik(link string) string {
	u, err := normalizeFreepikURL(link)
	if err != nil {
		return ""
	}

	hostParts := strings.Split(u.Hostname(), ".")
	if len(hostParts) < 3 {
		return "www"
	}
	return hostParts[0]
}

func GetDownloadLinkFreepik(link string) (string, error) {
	normalized, err := normalizeFreepikURL(link)
	if err != nil {
		return "", errs.Wrap(&err, "normalizeFreepikURL")
	}

	resourceID, err := extractResourceID(normalized)
	if err != nil {
		return "", errs.Wrap(&err, "extractResourceID")
	}

	cookieHeader, cookieSource, csrf, authToken, err := loadFreepikRequestAuth()
	if err != nil {
		return "", errs.Wrap(&err, "loadCookieHeader")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	assetType := detectAssetTypeFromPath(strings.ToLower(normalized.Path))
	if assetType == "3d" {
		return "", errs.New("3d format selection required")
	}
	var pageData *assetPageData
	if assetType == "icon" || assetType == "video" {
		pageData, _ = fetchAssetPageData(client, normalized, cookieHeader, cookieSource)
	}

	if assetType == "icon" {
		if directURL := pageData.bestIconURL(); directURL != "" {
			return directURL, nil
		}
	}
	if assetType == "video" {
		return getDownloadLinkFreepikVideo(client, normalized, resourceID, pageData, cookieHeader, cookieSource, csrf, authToken)
	}

	candidates := buildDownloadEndpoints(normalized, resourceID, pageData)
	failures := make([]string, 0, len(candidates))

	for _, candidate := range candidates {
		downloadURL, statusCode, body, reqErr := executeDownloadRequest(client, candidate.label, candidate.url, normalized.String(), cookieHeader, cookieSource, csrf, authToken)
		if reqErr == nil && downloadURL != "" {
			return downloadURL, nil
		}

		failures = append(failures, fmt.Sprintf("%s -> %s", candidate.label, summarizeResponseError(statusCode, body, reqErr)))
	}

	if directURL := pageData.bestFallbackURL(assetType); directURL != "" {
		return directURL, nil
	}

	return "", errs.New(strings.Join(failures, " | "))
}

func GetCacheableVideoDownloadLinkFreepik(link string, maxBytes int64) (string, error) {
	normalized, err := normalizeFreepikURL(link)
	if err != nil {
		return "", errs.Wrap(&err, "normalizeFreepikURL")
	}

	resourceID, err := extractResourceID(normalized)
	if err != nil {
		return "", errs.Wrap(&err, "extractResourceID")
	}

	cookieHeader, cookieSource, csrf, authToken, err := loadFreepikRequestAuth()
	if err != nil {
		return "", errs.Wrap(&err, "loadCookieHeader")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	pageData, err := fetchAssetPageData(client, normalized, cookieHeader, cookieSource)
	if err != nil {
		return "", errs.Wrap(&err, "fetchAssetPageData")
	}

	optionIDs := pageData.bestVideoCacheOptionIDs(resourceID, maxBytes)
	if len(optionIDs) == 0 {
		return "", errs.New("no cacheable video option fits the configured size limit")
	}

	return getDownloadLinkFreepikVideoWithOptionIDs(client, normalized, resourceID, pageData, optionIDs, false, cookieHeader, cookieSource, csrf, authToken)
}

func Get3DFormatOptionsFreepik(link string) ([]models.ThreeDFormatOption, error) {
	normalized, err := normalizeFreepikURL(link)
	if err != nil {
		return nil, errs.Wrap(&err, "normalizeFreepikURL")
	}
	if !is3DPath(strings.ToLower(normalized.Path)) {
		return nil, errs.New("not a 3d model link")
	}

	modelID, err := extractResourceID(normalized)
	if err != nil {
		return nil, errs.Wrap(&err, "extractResourceID")
	}

	cookieHeader, cookieSource, _, _, err := loadFreepikRequestAuth()
	if err != nil {
		return nil, errs.Wrap(&err, "loadCookieHeader")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	metadata, err := fetch3DModelMetadata(client, normalized, modelID, cookieHeader, cookieSource)
	if err != nil {
		return nil, errs.Wrap(&err, "fetch3DModelMetadata")
	}

	options := enabled3DFormatOptions(metadata)
	if len(options) == 0 {
		return nil, errs.New("no 3d formats available")
	}
	return options, nil
}

func GetDownloadLinkFreepik3D(link, fileType string) (string, error) {
	normalized, err := normalizeFreepikURL(link)
	if err != nil {
		return "", errs.Wrap(&err, "normalizeFreepikURL")
	}
	if !is3DPath(strings.ToLower(normalized.Path)) {
		return "", errs.New("not a 3d model link")
	}

	modelID, err := extractResourceID(normalized)
	if err != nil {
		return "", errs.Wrap(&err, "extractResourceID")
	}

	normalizedFileType := normalize3DFileType(fileType)
	if normalizedFileType == "" {
		return "", errs.New("unsupported 3d file type")
	}

	cookieHeader, cookieSource, csrf, authToken, err := loadFreepikRequestAuth()
	if err != nil {
		return "", errs.Wrap(&err, "loadCookieHeader")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	metadata, err := fetch3DModelMetadata(client, normalized, modelID, cookieHeader, cookieSource)
	if err != nil {
		return "", errs.Wrap(&err, "fetch3DModelMetadata")
	}
	if !is3DFileTypeAvailable(metadata, normalizedFileType) {
		return "", errs.New("selected 3d format is not available")
	}

	return getDownloadLinkFreepik3D(client, normalized, modelID, normalizedFileType, metadata, cookieHeader, cookieSource, csrf, authToken)
}

func GetDownloadLinkFreepikFreePsd(orgLink string) (string, error) {
	return GetDownloadLinkFreepik(orgLink)
}

func GetDownloadLinkFreepikFreePsd2(orgLink string) (string, error) {
	return GetDownloadLinkFreepik(orgLink)
}

func GetDownloadLinkFreepikPremiumVideo(orgLink string) (string, error) {
	return GetDownloadLinkFreepik(orgLink)
}

func GetDownloadLinkFreepikPremiumVideo2(orgLink string) (string, error) {
	return GetDownloadLinkFreepik(orgLink)
}

func GetDownloadLinkFreepikPremiumAiImage2(orgLink string) (string, error) {
	return GetDownloadLinkFreepik(orgLink)
}

func GetDownloadLinkFreepikPremiumAiImage(orgLink string) (string, error) {
	return GetDownloadLinkFreepik(orgLink)
}

func GetDownloadLinkFreepikIcon2(orgLink string) (string, error) {
	return GetDownloadLinkFreepik(orgLink)
}

func GetDownloadLinkFreepikIcon(orgLink string) (string, error) {
	return GetDownloadLinkFreepik(orgLink)
}

func GetFreepikAuthStatus() (*FreepikAuthStatus, error) {
	cookieHeader, source, err := loadCookieHeaderWithSource()
	if err != nil {
		return nil, errs.Wrap(&err, "loadCookieHeaderWithSource")
	}

	authToken := strings.TrimSpace(os.Getenv("FREEPIK_BEARER_TOKEN"))
	if authToken == "" {
		authToken = getCookieValue(cookieHeader, "GR_TOKEN")
	}

	status := &FreepikAuthStatus{Source: source}
	if authToken == "" {
		return status, nil
	}

	expiresAt, ok := parseJWTExpiry(authToken)
	if !ok {
		return nil, errs.New("could not decode GR_TOKEN expiry")
	}

	status.HasToken = true
	status.ExpiresAt = expiresAt
	return status, nil
}

func ForceRefreshFreepikAuth() (*FreepikAuthStatus, bool, error) {
	if strings.TrimSpace(os.Getenv("FREEPIK_COOKIE_HEADER")) != "" {
		status, err := GetFreepikAuthStatus()
		return status, false, err
	}

	cookieFile := strings.TrimSpace(os.Getenv("FREEPIK_COOKIES_FILE"))
	if cookieFile == "" {
		cookieFile = "freepik_cookies.json"
	}

	freepikAuthRefreshMu.Lock()
	defer freepikAuthRefreshMu.Unlock()

	cookieMap, format, err := loadCookieMapFromFile(cookieFile)
	if err != nil {
		return nil, false, errs.Wrap(&err, "loadCookieMapFromFile")
	}

	refreshedMap, refreshed, refreshErr := refreshFreepikCookieMap(cookieMap)
	if refreshErr != nil {
		status, statusErr := freepikAuthStatusFromCookieMap(cookieFile, cookieMap)
		if statusErr != nil {
			return nil, false, refreshErr
		}
		return status, false, refreshErr
	}

	if refreshed {
		if err := writeCookieMapToFile(cookieFile, format, refreshedMap); err != nil {
			return nil, false, errs.Wrap(&err, "writeCookieMapToFile")
		}
		cookieMap = refreshedMap
	}

	status, err := freepikAuthStatusFromCookieMap(cookieFile, cookieMap)
	if err != nil {
		return nil, refreshed, err
	}
	return status, refreshed, nil
}

func freepikAuthStatusFromCookieMap(source string, cookieMap map[string]string) (*FreepikAuthStatus, error) {
	status := &FreepikAuthStatus{Source: source}

	authToken := strings.TrimSpace(os.Getenv("FREEPIK_BEARER_TOKEN"))
	if authToken == "" {
		authToken = strings.TrimSpace(cookieMap["GR_TOKEN"])
	}
	if authToken == "" {
		return status, nil
	}

	expiresAt, ok := parseJWTExpiry(authToken)
	if !ok {
		return nil, errs.New("could not decode GR_TOKEN expiry")
	}

	status.HasToken = true
	status.ExpiresAt = expiresAt
	return status, nil
}

func normalizeFreepikURL(link string) (*url.URL, error) {
	v := strings.TrimSpace(link)
	if v == "" {
		return nil, errs.New("link is empty")
	}
	if !strings.Contains(v, "://") {
		v = "https://" + v
	}

	u, err := url.Parse(v)
	if err != nil {
		return nil, errs.Wrap(&err, "url.Parse")
	}

	if strings.Contains(u.Host, "facebook.com") {
		target := u.Query().Get("u")
		if target != "" {
			decoded, decErr := url.QueryUnescape(target)
			if decErr == nil {
				return normalizeFreepikURL(decoded)
			}
		}
	}

	if !isSupportedAssetHost(u.Hostname()) {
		return nil, errs.New("please send a magnific.com or freepik.com link")
	}

	if u.Scheme == "" {
		u.Scheme = "https"
	}

	return u, nil
}

func extractResourceID(u *url.URL) (string, error) {
	if id := resourceIDRegex.FindStringSubmatch(u.Path); len(id) > 1 {
		return id[1], nil
	}

	q := u.Query()
	if id := strings.TrimSpace(q.Get("resource")); id != "" {
		return id, nil
	}
	if id := strings.TrimSpace(q.Get("optionId")); id != "" {
		return id, nil
	}
	if id := strings.TrimSpace(q.Get("sku")); id != "" {
		return id, nil
	}
	if id := lastNumericPathToken(u.Path); id != "" {
		return id, nil
	}

	return "", errs.New("resource id not found")
}

func lastNumericPathToken(urlPath string) string {
	segments := strings.Split(strings.Trim(urlPath, "/"), "/")
	for i := len(segments) - 1; i >= 0; i-- {
		matches := numericTokenRegex.FindAllString(segments[i], -1)
		if len(matches) > 0 {
			return matches[len(matches)-1]
		}
	}
	return ""
}

func buildDownloadEndpoints(u *url.URL, resourceID string, pageData *assetPageData) []endpointCandidate {
	base := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	path := strings.ToLower(u.Path)
	locale := GetLanguageFreepik(u.String())
	regularType := ""
	if pageData != nil {
		regularType = strings.TrimSpace(pageData.RegularType)
	}
	if regularType == "" {
		regularType = regularResourceType(path)
	}

	switch {
	case isVideoPath(path):
		optionIDs := pageData.bestVideoOptionIDs(resourceID)
		if len(optionIDs) == 0 {
			optionIDs = []string{resourceID}
		}
		candidates := make([]endpointCandidate, 0, len(optionIDs)*4+1)
		for _, optionID := range optionIDs {
			candidates = append(candidates,
				endpointCandidate{label: fmt.Sprintf("video-option[%s]", optionID), url: fmt.Sprintf("%s/api/video/download?optionId=%s", base, optionID)},
				endpointCandidate{label: fmt.Sprintf("video-option-action[%s]", optionID), url: fmt.Sprintf("%s/api/video/download?optionId=%s&action=download", base, optionID)},
				endpointCandidate{label: fmt.Sprintf("video-resource-option[%s]", optionID), url: fmt.Sprintf("%s/api/video/download?resource=%s&optionId=%s", base, resourceID, optionID)},
				endpointCandidate{label: fmt.Sprintf("video-resource-option-action[%s]", optionID), url: fmt.Sprintf("%s/api/video/download?resource=%s&optionId=%s&action=download", base, resourceID, optionID)},
			)
		}
		candidates = append(candidates, endpointCandidate{label: "video-resource", url: fmt.Sprintf("%s/api/video/download?resource=%s", base, resourceID)})
		return uniqueCandidates(candidates)
	case isIconPath(path):
		return buildIconDownloadEndpoints(base, resourceID)
	default:
		candidates := []endpointCandidate{}
		if locale != "" {
			if regularType != "" {
				candidates = append(candidates,
					endpointCandidate{label: "regular-contentType-locale", url: fmt.Sprintf("%s/api/regular/download?resource=%s&action=download&contentType=%s&locale=%s", base, resourceID, regularType, locale)},
					endpointCandidate{label: "regular-type-locale", url: fmt.Sprintf("%s/api/regular/download?resource=%s&action=download&type=%s&locale=%s", base, resourceID, regularType, locale)},
				)
			}
			candidates = append(candidates,
				endpointCandidate{label: "regular-locale", url: fmt.Sprintf("%s/api/regular/download?resource=%s&action=download&locale=%s", base, resourceID, locale)},
				endpointCandidate{label: "regular-lang", url: fmt.Sprintf("%s/api/regular/download?resource=%s&action=download&lang=%s", base, resourceID, locale)},
			)
		}

		candidates = append(candidates,
			endpointCandidate{label: "regular-default", url: fmt.Sprintf("%s/api/regular/download?resource=%s&action=download", base, resourceID)},
			endpointCandidate{label: "regular-resource-only", url: fmt.Sprintf("%s/api/regular/download?resource=%s", base, resourceID)},
			endpointCandidate{label: "regular-option", url: fmt.Sprintf("%s/api/regular/download?optionId=%s&action=download", base, resourceID)},
			endpointCandidate{label: "regular-id", url: fmt.Sprintf("%s/api/regular/download?id=%s&action=download", base, resourceID)},
			endpointCandidate{label: "regular-resource-id", url: fmt.Sprintf("%s/api/regular/download?resource_id=%s&action=download", base, resourceID)},
		)

		if regularType != "" {
			candidates = append(candidates,
				endpointCandidate{label: "regular-type", url: fmt.Sprintf("%s/api/regular/download?resource=%s&action=download&type=%s", base, resourceID, regularType)},
				endpointCandidate{label: "regular-contentType", url: fmt.Sprintf("%s/api/regular/download?resource=%s&action=download&contentType=%s", base, resourceID, regularType)},
				endpointCandidate{label: "regular-content_type", url: fmt.Sprintf("%s/api/regular/download?resource=%s&action=download&content_type=%s", base, resourceID, regularType)},
			)
		}

		return uniqueCandidates(candidates)
	}
}

func buildIconDownloadEndpoints(base, resourceID string) []endpointCandidate {
	return uniqueCandidates([]endpointCandidate{
		{label: "icon-svg-original", url: fmt.Sprintf("%s/api/icon/download?optionId=%s&format=svg&type=original", base, resourceID)},
		{label: "icon-svg-copy", url: fmt.Sprintf("%s/api/icon/download?optionId=%s&format=svg&type=copy", base, resourceID)},
	})
}

func regularResourceType(path string) string {
	switch {
	case strings.Contains(path, "psd"):
		return "psd"
	case strings.Contains(path, "photo"), strings.Contains(path, "foto"), strings.Contains(path, "fotos"):
		return "photo"
	case strings.Contains(path, "vector"), strings.Contains(path, "vecteur"), strings.Contains(path, "vectorial"):
		return "vector"
	case strings.Contains(path, "ai-image"):
		return "ai"
	default:
		return ""
	}
}

func uniqueCandidates(items []endpointCandidate) []endpointCandidate {
	seen := make(map[string]struct{}, len(items))
	out := make([]endpointCandidate, 0, len(items))
	for _, item := range items {
		if item.url == "" {
			continue
		}
		if _, ok := seen[item.url]; ok {
			continue
		}
		seen[item.url] = struct{}{}
		out = append(out, item)
	}
	return out
}

func detectAssetTypeFromPath(path string) string {
	switch {
	case isIconPath(path):
		return "icon"
	case isVideoPath(path):
		return "video"
	case is3DPath(path):
		return "3d"
	default:
		return "regular"
	}
}

func isIconPath(path string) bool {
	return strings.Contains(path, "/icon/") || strings.Contains(path, "/stock-icon/")
}

func isVideoPath(path string) bool {
	switch {
	case strings.Contains(path, "/premium-video/"):
		return true
	case strings.Contains(path, "/free-video/"):
		return true
	case strings.Contains(path, "/video/"):
		return true
	case strings.Contains(path, "/motion-graphics/"):
		return true
	case strings.Contains(path, "/premium-motion-graphics/"):
		return true
	case strings.Contains(path, "/free-motion-graphics/"):
		return true
	case strings.Contains(path, "/stock-video/"):
		return true
	case strings.Contains(path, "/footage/"):
		return true
	default:
		return false
	}
}

func is3DPath(path string) bool {
	return strings.Contains(path, "/3d-model/") || strings.Contains(path, "3d-models")
}

func loadFreepikRequestAuth() (cookieHeader, cookieSource, csrf, authToken string, err error) {
	cookieHeader, cookieSource, err = loadCookieHeaderWithSource()
	if err != nil {
		return "", "", "", "", err
	}

	csrf = strings.TrimSpace(os.Getenv("FREEPIK_CSRF_TOKEN"))
	if csrf == "" {
		csrf = getCookieValue(cookieHeader, "csrf_freepik")
	}
	if csrf == "" {
		csrf = getCookieValue(cookieHeader, "csrftoken")
	}

	authToken = strings.TrimSpace(os.Getenv("FREEPIK_BEARER_TOKEN"))
	if authToken == "" {
		authToken = getCookieValue(cookieHeader, "GR_TOKEN")
	}
	return cookieHeader, cookieSource, csrf, authToken, nil
}

func fetch3DModelMetadata(client *http.Client, pageURL *url.URL, modelID, cookieHeader, cookieSource string) (*model3DMetadata, error) {
	if client == nil || pageURL == nil {
		return nil, errs.New("3d metadata request is missing client or url")
	}

	endpoint := fmt.Sprintf(
		"%s://%s/api/model3d?id=%s&locale=%s",
		pageURL.Scheme,
		pageURL.Host,
		url.QueryEscape(modelID),
		url.QueryEscape(model3DLocale(pageURL)),
	)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errs.Wrap(&err, "http.NewRequest")
	}

	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cookie", cookieHeader)
	req.Header.Set("Referer", pageURL.String())
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:148.0) Gecko/20100101 Firefox/148.0")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := client.Do(req)
	if err != nil {
		return nil, errs.Wrap(&err, "client.Do")
	}
	defer resp.Body.Close()

	if persistErr := persistAuthCookiesFromResponse(cookieSource, resp.Cookies()); persistErr != nil {
		log.Printf("persist 3d metadata auth cookies failed: %v", persistErr)
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, errs.Wrap(&err, "readResponseBody")
	}
	if resp.StatusCode >= 300 {
		return nil, errs.New(summarizeResponseError(resp.StatusCode, body, nil))
	}

	var metadata model3DMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return nil, errs.Wrap(&err, "json.Unmarshal")
	}
	return &metadata, nil
}

func enabled3DFormatOptions(metadata *model3DMetadata) []models.ThreeDFormatOption {
	all := build3DFormatOptions(metadata)
	options := make([]models.ThreeDFormatOption, 0, len(all))
	for _, option := range all {
		if option.Enabled {
			options = append(options, option)
		}
	}
	return options
}

func build3DFormatOptions(metadata *model3DMetadata) []models.ThreeDFormatOption {
	if metadata == nil {
		return nil
	}

	return []models.ThreeDFormatOption{
		{ID: 1, Name: "BLEND", FileType: "blend", Enabled: metadata.HasBlendFile},
		{ID: 2, Name: "OBJ", FileType: "obj", Enabled: metadata.HasObjFile},
		{ID: 3, Name: "FBX", FileType: "fbx", Enabled: metadata.HasFbxFile},
		{ID: 4, Name: "TEXTURES", FileType: "textures", Enabled: metadata.Specifications.IncludeTextures},
	}
}

func normalize3DFileType(fileType string) string {
	switch strings.ToLower(strings.TrimSpace(fileType)) {
	case "blend":
		return "blend"
	case "obj":
		return "obj"
	case "fbx":
		return "fbx"
	case "textures":
		return "textures"
	default:
		return ""
	}
}

func is3DFileTypeAvailable(metadata *model3DMetadata, fileType string) bool {
	for _, option := range build3DFormatOptions(metadata) {
		if option.FileType == fileType {
			return option.Enabled
		}
	}
	return false
}

func getDownloadLinkFreepik3D(client *http.Client, normalized *url.URL, modelID, fileType string, metadata *model3DMetadata, cookieHeader, cookieSource, csrf, authToken string) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if normalized == nil {
		return "", errs.New("3d url is nil")
	}

	endpoint := build3DDownloadEndpoint(normalized, modelID, fileType, metadata)
	downloadURL, statusCode, body, reqErr := executeDownloadRequest(client, "3d-"+fileType, endpoint, normalized.String(), cookieHeader, cookieSource, csrf, authToken)
	if reqErr == nil && downloadURL != "" {
		return downloadURL, nil
	}
	return "", errs.New(fmt.Sprintf("3d-download[%s] -> %s", fileType, summarizeResponseError(statusCode, body, reqErr)))
}

func build3DDownloadEndpoint(normalized *url.URL, modelID, fileType string, metadata *model3DMetadata) string {
	query := url.Values{}
	query.Set("fileType", fileType)

	if walletID := normalizeScalarString(metadata.WalletID); walletID != "" {
		query.Set("walletId", walletID)
	}
	if searchExpression := strings.TrimSpace(metadata.SearchExpression); searchExpression != "" {
		query.Set("searchExpression", searchExpression)
	}

	return fmt.Sprintf("%s://%s/api/model3d/%s/download?%s", normalized.Scheme, normalized.Host, modelID, query.Encode())
}

func normalizeScalarString(value interface{}) string {
	switch current := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(current)
	case float64:
		return strconv.FormatInt(int64(current), 10)
	case int:
		return strconv.Itoa(current)
	case int64:
		return strconv.FormatInt(current, 10)
	case json.Number:
		return current.String()
	default:
		return strings.TrimSpace(fmt.Sprint(current))
	}
}

func model3DLocale(u *url.URL) string {
	switch locale := strings.ToLower(strings.TrimSpace(GetLanguageFreepik(u.String()))); locale {
	case "", "www":
		return "en"
	default:
		return locale
	}
}

func fetchAssetPageData(client *http.Client, pageURL *url.URL, cookieHeader, cookieSource string) (*assetPageData, error) {
	if client == nil || pageURL == nil {
		return nil, nil
	}

	req, err := http.NewRequest(http.MethodGet, pageURL.String(), nil)
	if err != nil {
		return nil, errs.Wrap(&err, "http.NewRequest")
	}

	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Cookie", cookieHeader)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, errs.Wrap(&err, "client.Do")
	}
	defer resp.Body.Close()

	if persistErr := persistAuthCookiesFromResponse(cookieSource, resp.Cookies()); persistErr != nil {
		log.Printf("persist asset page auth cookies failed: %v", persistErr)
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, errs.Wrap(&err, "readResponseBody")
	}

	data := &assetPageData{}
	videoURLs := extractEmbeddedVideoURLs(body)
	optionIDs := extractEmbeddedVideoOptionIDs(body)

	match := nextDataRegex.FindSubmatch(body)
	if len(match) >= 2 {
		var payload struct {
			Props struct {
				PageProps struct {
					Icon        *iconPageData `json:"icon"`
					ID          int           `json:"id"`
					Premium     bool          `json:"premium"`
					Orientation string        `json:"orientation"`
					VideoSrc    string        `json:"videoSrc"`
					Previews    []mediaURL    `json:"previews"`
					Options     []videoOption `json:"options"`
					RegularType string        `json:"regularType"`
				} `json:"pageProps"`
			} `json:"props"`
		}
		if err := json.Unmarshal(match[1], &payload); err == nil {
			pageProps := payload.Props.PageProps
			if pageProps.Icon != nil && pageProps.Icon.ID != 0 {
				data.Icon = pageProps.Icon
			}
			if pageProps.ID != 0 || pageProps.VideoSrc != "" || len(pageProps.Previews) > 0 || len(pageProps.Options) > 0 {
				data.Video = &videoPageData{
					ID:          pageProps.ID,
					Premium:     pageProps.Premium,
					Orientation: pageProps.Orientation,
					VideoSrc:    pageProps.VideoSrc,
					Previews:    pageProps.Previews,
					Options:     pageProps.Options,
					URLs:        nil,
					OptionIDs:   nil,
				}
			}
			data.RegularType = strings.TrimSpace(pageProps.RegularType)
		}
	}

	if len(videoURLs) > 0 || len(optionIDs) > 0 {
		if data.Video == nil {
			data.Video = &videoPageData{}
		}
		data.Video.URLs = appendUniqueStrings(data.Video.URLs, videoURLs...)
		data.Video.OptionIDs = appendUniqueStrings(data.Video.OptionIDs, optionIDs...)
	}

	if data.Icon == nil && data.Video == nil {
		return nil, nil
	}
	return data, nil
}

func (d *assetPageData) bestFallbackURL(assetType string) string {
	if d == nil {
		return ""
	}
	switch assetType {
	case "icon":
		return ""
	case "video":
		return d.bestVideoURL()
	default:
		return ""
	}
}

func (d *assetPageData) bestIconURL() string {
	if d == nil || d.Icon == nil {
		return ""
	}

	candidates := []string{
		strings.TrimSpace(d.Icon.Thumbnails.Large.URL),
		strings.TrimSpace(d.Icon.Thumbnails.Medium.URL),
		strings.TrimSpace(d.Icon.Thumbnails.Small.URL),
	}
	for _, candidate := range candidates {
		if looksLikeSVGURL(candidate) {
			return candidate
		}
	}
	return ""
}

func (d *assetPageData) bestVideoURL() string {
	if d == nil || d.Video == nil {
		return ""
	}

	candidates := make([]string, 0, 1+len(d.Video.Previews)+len(d.Video.URLs))
	if strings.TrimSpace(d.Video.VideoSrc) != "" {
		candidates = append(candidates, strings.TrimSpace(d.Video.VideoSrc))
	}
	for _, preview := range d.Video.Previews {
		candidates = append(candidates, strings.TrimSpace(preview.URL))
	}
	candidates = append(candidates, d.Video.URLs...)

	best := ""
	bestScore := math.MinInt
	for _, candidate := range candidates {
		score := scoreCandidateURL(candidate, "video-page")
		if score > bestScore {
			best = candidate
			bestScore = score
		}
	}
	if bestScore == math.MinInt {
		return ""
	}
	return strings.TrimSpace(best)
}

func mediaScore(item mediaURL) int {
	score := item.Width * item.Height
	urlValue := strings.ToLower(item.URL)
	if strings.Contains(urlValue, "/clear/") {
		score += 1_000_000_000
	}
	if strings.Contains(urlValue, "/watermarked/") {
		score -= 1_000_000_000
	}
	return score
}

func (d *assetPageData) bestVideoOptionIDs(resourceID string) []string {
	if d == nil || d.Video == nil {
		return nil
	}

	ordered := make([]string, 0, len(d.Video.Options)+len(d.Video.OptionIDs))
	seen := map[string]struct{}{}

	type scoredID struct {
		id    string
		score int
	}
	scored := make([]scoredID, 0, len(d.Video.Options))
	for _, option := range d.Video.Options {
		if option.ID == 0 {
			continue
		}
		score := option.Width * option.Height
		if strings.EqualFold(option.Container, "mov") {
			score += 2_000_000
		}
		if strings.EqualFold(option.Container, "mp4") {
			score += 1_000_000
		}
		if option.IsOriginal {
			score += 500_000
		}
		if option.Active {
			score += 100_000
		}
		scored = append(scored, scoredID{id: fmt.Sprintf("%d", option.ID), score: score})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})
	for _, item := range scored {
		if item.id == "" || item.id == resourceID {
			continue
		}
		if _, ok := seen[item.id]; ok {
			continue
		}
		seen[item.id] = struct{}{}
		ordered = append(ordered, item.id)
	}

	for _, id := range d.Video.OptionIDs {
		id = strings.TrimSpace(id)
		if id == "" || id == resourceID {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ordered = append(ordered, id)
	}

	return ordered
}

func (d *assetPageData) bestVideoCacheOptionIDs(resourceID string, maxBytes int64) []string {
	if d == nil || d.Video == nil {
		return nil
	}
	if maxBytes <= 0 {
		return d.bestVideoOptionIDs(resourceID)
	}

	type scoredID struct {
		id    string
		score int
	}

	scored := make([]scoredID, 0, len(d.Video.Options))
	for _, option := range d.Video.Options {
		if option.ID == 0 || fmt.Sprintf("%d", option.ID) == resourceID {
			continue
		}
		if !isCacheableVideoContainer(option.Container) {
			continue
		}
		if est := optionEstimatedBytes(option); est > 0 && est > maxBytes {
			continue
		}

		score := option.Width * option.Height
		if option.Active {
			score += 500_000
		}
		if option.IsOriginal {
			score += 200_000
		}
		if strings.EqualFold(option.Container, "mp4") {
			score += 150_000
		}
		if strings.EqualFold(option.Container, "mov") {
			score += 100_000
		}
		scored = append(scored, scoredID{
			id:    fmt.Sprintf("%d", option.ID),
			score: score,
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	ordered := make([]string, 0, len(scored))
	for _, item := range scored {
		ordered = append(ordered, item.id)
	}
	return appendUniqueStrings(nil, ordered...)
}

func isCacheableVideoContainer(container string) bool {
	switch strings.ToLower(strings.TrimSpace(container)) {
	case "", "mp4", "mov", "webm", "avi", "zip":
		return true
	default:
		return false
	}
}

func optionEstimatedBytes(option videoOption) int64 {
	if option.Size <= 0 {
		return 0
	}
	return int64(option.Size) * 1024 * 1024
}

func loadCookieHeader() (string, error) {
	header, _, err := loadCookieHeaderWithSource()
	return header, err
}

func loadCookieHeaderWithSource() (string, string, error) {
	raw := strings.TrimSpace(os.Getenv("FREEPIK_COOKIE_HEADER"))
	if raw != "" {
		return raw, "env:FREEPIK_COOKIE_HEADER", nil
	}

	cookieFile := strings.TrimSpace(os.Getenv("FREEPIK_COOKIES_FILE"))
	if cookieFile == "" {
		cookieFile = "freepik_cookies.json"
	}

	cookieMap, format, err := loadCookieMapFromFile(cookieFile)
	if err != nil {
		return "", "", errs.New("set FREEPIK_COOKIE_HEADER or provide freepik_cookies.json")
	}

	cookieMap, err = ensureFreshCookieMap(cookieFile, format, cookieMap)
	if err != nil {
		return "", "", errs.Wrap(&err, "ensureFreshCookieMap")
	}

	return joinCookieMap(cookieMap), cookieFile, nil
}

func loadCookieMapFromFile(cookieFile string) (map[string]string, string, error) {
	data, err := os.ReadFile(cookieFile)
	if err != nil {
		return nil, "", err
	}

	var payload struct {
		RequestCookies map[string]string `json:"Request Cookies"`
		Cookies        map[string]string `json:"cookies"`
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, "", errs.Wrap(&err, "json.Unmarshal")
	}

	if len(payload.RequestCookies) > 0 {
		return cloneCookieMap(payload.RequestCookies), "request", nil
	}
	if len(payload.Cookies) > 0 {
		return cloneCookieMap(payload.Cookies), "cookies", nil
	}

	flatMap := map[string]string{}
	if err := json.Unmarshal(data, &flatMap); err == nil && len(flatMap) > 0 {
		return flatMap, "flat", nil
	}

	return nil, "", errs.New("cookie map is empty")
}

func ensureFreshCookieMap(cookieFile, format string, cookieMap map[string]string) (map[string]string, error) {
	if !shouldAutoRefreshFreepikAuth(cookieMap) {
		return cookieMap, nil
	}

	freepikAuthRefreshMu.Lock()
	defer freepikAuthRefreshMu.Unlock()

	latestMap, latestFormat, err := loadCookieMapFromFile(cookieFile)
	if err == nil {
		cookieMap = latestMap
		if latestFormat != "" {
			format = latestFormat
		}
		if !shouldAutoRefreshFreepikAuth(cookieMap) {
			return cookieMap, nil
		}
	}

	refreshedMap, refreshed, err := refreshFreepikCookieMap(cookieMap)
	if err != nil {
		log.Printf("freepik auth refresh failed: %v", err)
		return cookieMap, nil
	}
	if !refreshed {
		return cookieMap, nil
	}
	if err := writeCookieMapToFile(cookieFile, format, refreshedMap); err != nil {
		return nil, err
	}
	return refreshedMap, nil
}

func shouldAutoRefreshFreepikAuth(cookieMap map[string]string) bool {
	if len(cookieMap) == 0 {
		return false
	}
	if strings.TrimSpace(cookieMap["GR_REFRESH"]) == "" {
		return false
	}

	token := strings.TrimSpace(cookieMap["GR_TOKEN"])
	if token == "" {
		return true
	}

	expiresAt, ok := parseJWTExpiry(token)
	if !ok {
		return true
	}

	return time.Until(expiresAt) <= freepikAuthRefreshBefore()
}

func freepikAuthRefreshBefore() time.Duration {
	value := strings.TrimSpace(os.Getenv("FREEPIK_AUTH_REFRESH_BEFORE_MINUTES"))
	if value == "" {
		return 15 * time.Minute
	}

	minutes, err := strconv.Atoi(value)
	if err != nil || minutes <= 0 {
		return 15 * time.Minute
	}
	return time.Duration(minutes) * time.Minute
}

func refreshFreepikCookieMap(cookieMap map[string]string) (map[string]string, bool, error) {
	failures := make([]string, 0, 3)

	updated, refreshed, err := refreshFreepikCookieMapViaSecureToken(cookieMap)
	if err == nil {
		return updated, refreshed, nil
	}
	failures = append(failures, "securetoken: "+err.Error())

	updated, refreshed, err = refreshFreepikCookieMapViaHomepage(cookieMap)
	if err == nil {
		return updated, refreshed, nil
	}
	failures = append(failures, "homepage: "+err.Error())

	updated, refreshed, err = refreshFreepikCookieMapViaBrowser(cookieMap)
	if err == nil {
		return updated, refreshed, nil
	}
	failures = append(failures, "browser: "+err.Error())

	return nil, false, errs.New(strings.Join(failures, " | "))
}

func refreshFreepikCookieMapViaHomepage(cookieMap map[string]string) (map[string]string, bool, error) {
	req, err := http.NewRequest(http.MethodGet, freepikAuthRefreshURL, nil)
	if err != nil {
		return nil, false, errs.Wrap(&err, "http.NewRequest")
	}

	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Cookie", joinCookieMap(cookieMap))
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	client := freepikAuthClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, errs.Wrap(&err, "client.Do")
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, false, errs.Wrap(&err, "readResponseBody")
	}

	updated := cloneCookieMap(cookieMap)
	refreshed := false
	for _, item := range resp.Cookies() {
		switch item.Name {
		case "GR_TOKEN", "GR_REFRESH", "FP_TE", "csrftoken", "csrf_freepik":
			if strings.TrimSpace(item.Value) == "" {
				continue
			}
			updated[item.Name] = item.Value
			refreshed = true
		}
	}

	if resp.StatusCode >= 400 {
		return nil, false, errs.New(summarizeResponseError(resp.StatusCode, body, err))
	}
	if !refreshed {
		return nil, false, errs.New(summarizeResponseError(resp.StatusCode, body, err))
	}
	return updated, true, nil
}

func refreshFreepikCookieMapViaSecureToken(cookieMap map[string]string) (map[string]string, bool, error) {
	refreshToken := strings.TrimSpace(cookieMap["GR_REFRESH"])
	if refreshToken == "" {
		return nil, false, errs.New("GR_REFRESH is missing")
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)

	req, err := http.NewRequest(http.MethodPost, secureTokenRefreshRequestURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, false, errs.Wrap(&err, "http.NewRequest")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	client := freepikSecureTokenClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, false, errs.Wrap(&err, "client.Do")
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, false, errs.Wrap(&err, "readResponseBody")
	}
	if resp.StatusCode >= 400 {
		return nil, false, errs.New(summarizeResponseError(resp.StatusCode, body, nil))
	}

	var payload struct {
		IDToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    string `json:"expires_in"`
		Error        struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, false, errs.Wrap(&err, "json.Unmarshal")
	}

	if strings.TrimSpace(payload.Error.Message) != "" {
		return nil, false, errs.New(payload.Error.Message)
	}
	if strings.TrimSpace(payload.IDToken) == "" {
		return nil, false, errs.New("securetoken refresh returned empty id_token")
	}

	updated := cloneCookieMap(cookieMap)
	updated["GR_TOKEN"] = strings.TrimSpace(payload.IDToken)
	if strings.TrimSpace(payload.RefreshToken) != "" {
		updated["GR_REFRESH"] = strings.TrimSpace(payload.RefreshToken)
	}
	if expiresInSeconds, convErr := strconv.Atoi(strings.TrimSpace(payload.ExpiresIn)); convErr == nil && expiresInSeconds > 0 {
		updated["FP_TE"] = strconv.FormatInt(time.Now().Add(time.Duration(expiresInSeconds)*time.Second).Unix(), 10)
	}
	return updated, true, nil
}

func secureTokenRefreshRequestURL() string {
	raw := strings.TrimSpace(freepikSecureTokenRefreshURL)
	apiKey := strings.TrimSpace(os.Getenv("FREEPIK_FIREBASE_API_KEY"))
	if raw == "" || apiKey == "" {
		return raw
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	query := parsed.Query()
	if query.Get("key") == "" {
		query.Set("key", apiKey)
		parsed.RawQuery = query.Encode()
	}
	return parsed.String()
}

func writeCookieMapToFile(cookieFile, format string, cookieMap map[string]string) error {
	var (
		data []byte
		err  error
	)

	switch format {
	case "cookies":
		payload := struct {
			Cookies map[string]string `json:"cookies"`
		}{Cookies: cookieMap}
		data, err = json.MarshalIndent(payload, "", "  ")
	case "flat":
		data, err = json.MarshalIndent(cookieMap, "", "  ")
	default:
		payload := struct {
			RequestCookies map[string]string `json:"Request Cookies"`
		}{RequestCookies: cookieMap}
		data, err = json.MarshalIndent(payload, "", "  ")
	}
	if err != nil {
		return errs.Wrap(&err, "json.MarshalIndent")
	}

	tmpPath := cookieFile + ".tmp"
	if err := os.WriteFile(tmpPath, append(data, '\n'), 0o600); err != nil {
		return errs.Wrap(&err, "os.WriteFile")
	}
	if err := os.Rename(tmpPath, cookieFile); err != nil {
		return errs.Wrap(&err, "os.Rename")
	}
	return nil
}

func persistAuthCookiesFromResponse(source string, items []*http.Cookie) error {
	if strings.TrimSpace(source) == "" || strings.HasPrefix(source, "env:") || len(items) == 0 {
		return nil
	}

	updates := map[string]string{}
	for _, item := range items {
		if item == nil {
			continue
		}
		if shouldPersistFreepikCookie(item.Name) && strings.TrimSpace(item.Value) != "" {
			updates[item.Name] = item.Value
		}
	}
	if len(updates) == 0 {
		return nil
	}

	freepikAuthRefreshMu.Lock()
	defer freepikAuthRefreshMu.Unlock()

	cookieMap, format, err := loadCookieMapFromFile(source)
	if err != nil {
		return err
	}

	changed := false
	for key, value := range updates {
		if cookieMap[key] != value {
			cookieMap[key] = value
			changed = true
		}
	}
	if !changed {
		return nil
	}

	return writeCookieMapToFile(source, format, cookieMap)
}

func shouldPersistFreepikCookie(name string) bool {
	switch strings.TrimSpace(name) {
	case "GR_TOKEN",
		"GR_REFRESH",
		"FP_TE",
		"csrftoken",
		"csrf_freepik",
		"sessionid",
		"fp_bot_check",
		"ak_bmsc",
		"UID",
		"GRID",
		"FP_MBL",
		"FP_MBL_NEW",
		"GR_LGURI":
		return true
	default:
		return false
	}
}

func joinCookieMap(cookieMap map[string]string) string {
	keys := make([]string, 0, len(cookieMap))
	for k := range cookieMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, cookieMap[k]))
	}

	return strings.Join(parts, "; ")
}

func cloneCookieMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}

	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func isAuthMissingOrExpired(cookieMap map[string]string) bool {
	token := strings.TrimSpace(cookieMap["GR_TOKEN"])
	if token == "" {
		return true
	}
	return isJWTExpired(token)
}

func getDownloadLinkFreepikVideo(client *http.Client, normalized *url.URL, videoID string, pageData *assetPageData, cookieHeader, cookieSource, csrf, authToken string) (string, error) {
	optionIDs := pageData.bestVideoOptionIDs(videoID)
	return getDownloadLinkFreepikVideoWithOptionIDs(client, normalized, videoID, pageData, optionIDs, true, cookieHeader, cookieSource, csrf, authToken)
}

func getDownloadLinkFreepikVideoWithOptionIDs(client *http.Client, normalized *url.URL, videoID string, pageData *assetPageData, optionIDs []string, includeDefault bool, cookieHeader, cookieSource, csrf, authToken string) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if normalized == nil {
		return "", errs.New("video url is nil")
	}

	hosts := []string{}
	if normalized.Host != "" {
		hosts = append(hosts, normalized.Host)
	}
	hosts = append(hosts, defaultAssetHosts()...)
	hosts = appendUniqueStrings(nil, hosts...)

	orientation := ""
	if pageData != nil && pageData.Video != nil {
		orientation = strings.TrimSpace(pageData.Video.Orientation)
	}

	failures := make([]string, 0, len(hosts)*4)
	for _, host := range hosts {
		base := fmt.Sprintf("https://%s/api/video/%s/download", host, videoID)
		candidates := make([]endpointCandidate, 0, len(optionIDs)*2+2)
		for _, optionID := range optionIDs {
			if optionID == "" {
				continue
			}
			if orientation != "" {
				candidates = append(candidates, endpointCandidate{
					label: fmt.Sprintf("video-detail-orientation-option[%s@%s]", optionID, host),
					url:   fmt.Sprintf("%s?orientation=%s&optionId=%s", base, url.QueryEscape(orientation), url.QueryEscape(optionID)),
				})
			}
			candidates = append(candidates, endpointCandidate{
				label: fmt.Sprintf("video-detail-option[%s@%s]", optionID, host),
				url:   fmt.Sprintf("%s?optionId=%s", base, url.QueryEscape(optionID)),
			})
		}
		if includeDefault {
			if orientation != "" {
				candidates = append(candidates, endpointCandidate{
					label: fmt.Sprintf("video-detail-orientation[%s]", host),
					url:   fmt.Sprintf("%s?orientation=%s", base, url.QueryEscape(orientation)),
				})
			}
			candidates = append(candidates, endpointCandidate{
				label: fmt.Sprintf("video-detail-default[%s]", host),
				url:   base,
			})
		}

		for _, candidate := range uniqueCandidates(candidates) {
			downloadURL, statusCode, body, reqErr := executeDownloadRequest(client, candidate.label, candidate.url, normalized.String(), cookieHeader, cookieSource, csrf, authToken)
			if reqErr == nil && downloadURL != "" {
				return downloadURL, nil
			}
			failures = append(failures, fmt.Sprintf("%s -> %s", candidate.label, summarizeResponseError(statusCode, body, reqErr)))
		}
	}

	return "", errs.New(strings.Join(failures, " | "))
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	var reader io.ReadCloser
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, errs.Wrap(&err, "gzip.NewReader")
		}
		defer gz.Close()
		reader = gz
	default:
		reader = resp.Body
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, errs.Wrap(&err, "io.ReadAll")
	}

	return body, nil
}

func extractDownloadURL(body []byte, context string) (string, error) {
	var decoded interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", errs.Wrap(&err, "json.Unmarshal")
	}

	if found := findBestURL(decoded, context); found != "" {
		return found, nil
	}

	return "", errs.New("download url not found in response")
}

func executeDownloadRequest(client *http.Client, context, endpoint, referer, cookieHeader, cookieSource, csrf, authToken string) (string, int, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", 0, nil, errs.Wrap(&err, "http.NewRequest")
	}

	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Cookie", cookieHeader)
	req.Header.Set("Priority", "u=4")
	req.Header.Set("Referer", referer)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:148.0) Gecko/20100101 Firefox/148.0")
	if csrf != "" {
		req.Header.Set("x-csrf-token", csrf)
		req.Header.Set("x-csrftoken", csrf)
	}
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", 0, nil, errs.Wrap(&err, "client.Do")
	}
	defer resp.Body.Close()

	if persistErr := persistAuthCookiesFromResponse(cookieSource, resp.Cookies()); persistErr != nil {
		log.Printf("persist auth cookies failed: %v", persistErr)
	}

	if directURL := extractDirectDownloadURL(resp, endpoint); directURL != "" {
		return directURL, resp.StatusCode, nil, nil
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return "", resp.StatusCode, nil, errs.Wrap(&err, "readResponseBody")
	}

	downloadURL, extractErr := extractDownloadURL(body, context)
	if extractErr != nil {
		return "", resp.StatusCode, body, extractErr
	}
	if resp.StatusCode >= 300 {
		return "", resp.StatusCode, body, errs.New(fmt.Sprintf("freepik response status=%d", resp.StatusCode))
	}

	return downloadURL, resp.StatusCode, body, nil
}

func summarizeResponseError(statusCode int, body []byte, reqErr error) string {
	if reqErr != nil && len(body) == 0 {
		return reqErr.Error()
	}

	bodyText := strings.TrimSpace(string(body))
	if bodyText == "" {
		if reqErr != nil {
			return reqErr.Error()
		}
		return fmt.Sprintf("status=%d empty body", statusCode)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err == nil {
		if message, ok := payload["error"].(string); ok && message != "" {
			return fmt.Sprintf("%s (status=%d)", message, statusCode)
		}
		if message, ok := payload["message"].(string); ok && message != "" {
			return fmt.Sprintf("%s (status=%d)", message, statusCode)
		}
	}

	if strings.Contains(strings.ToLower(bodyText), "access denied") {
		return fmt.Sprintf("access denied (status=%d)", statusCode)
	}

	bodyText = strings.Join(strings.Fields(bodyText), " ")
	if len(bodyText) > 160 {
		bodyText = bodyText[:160] + "..."
	}
	return fmt.Sprintf("%s (status=%d)", bodyText, statusCode)
}

func findBestURL(v interface{}, context string) string {
	values := make(map[string]struct{})
	collectURLs(v, values)
	if len(values) == 0 {
		return ""
	}

	urls := make([]string, 0, len(values))
	for raw := range values {
		urls = append(urls, raw)
	}
	sort.Strings(urls)

	best := ""
	bestScore := math.MinInt
	for _, raw := range urls {
		score := scoreCandidateURL(raw, context)
		if score > bestScore {
			best = raw
			bestScore = score
		}
	}
	if bestScore == math.MinInt {
		return ""
	}
	return best
}

func collectURLs(v interface{}, out map[string]struct{}) {
	switch t := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			collectURLs(t[k], out)
		}
	case []interface{}:
		for _, item := range t {
			collectURLs(item, out)
		}
	case string:
		raw := strings.TrimSpace(t)
		if strings.HasPrefix(strings.ToLower(raw), "http://") || strings.HasPrefix(strings.ToLower(raw), "https://") {
			out[raw] = struct{}{}
		}
	}
}

func scoreCandidateURL(raw, context string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return math.MinInt
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return math.MinInt
	}

	host := strings.ToLower(parsed.Hostname())
	urlPath := strings.ToLower(parsed.Path)
	ext := effectiveDownloadExtension(parsed)
	kind := responseContextKind(context)

	score := 0
	if looksLikeDownloadURL(raw) {
		score += 1000
	}

	switch {
	case strings.Contains(host, "downloadscdn"):
		score += 5000
	case strings.Contains(host, "videocdn.cdnpk.net"):
		score += 5000
	case strings.Contains(host, "gettyimages.com"):
		score += 5000
	case strings.Contains(host, "audiocdn.cdnpk.net"):
		score += 4500
	case strings.Contains(host, "cdn-icons.flaticon.com"), strings.Contains(host, "flaticon.com"):
		score += 4000
	case strings.Contains(host, "3d.cdnpk.net"):
		score += 4000
	}

	switch kind {
	case "video":
		if !isActualVideoDownloadURL(raw) {
			return math.MinInt
		}
		switch ext {
		case ".mov":
			score += 9000
		case ".mp4", ".webm", ".avi", ".zip":
			score += 7000
		case ".svg", ".png":
			score -= 10000
		}
		if strings.Contains(urlPath, "/downloads/original") {
			score += 12000
		} else if strings.Contains(urlPath, "/downloads/") {
			score += 7000
		}
		if strings.Contains(urlPath, "/previews/") {
			score -= 9000
		}
		if strings.Contains(urlPath, "/clear/") {
			score -= 3000
		}
		if strings.Contains(urlPath, "/watermarked/") {
			score -= 12000
		}
		if strings.Contains(host, "cdn-icons.flaticon.com") || strings.Contains(host, "flaticon.com") {
			score -= 12000
		}
	case "icon":
		if ext == ".svg" {
			score += 9000
		}
		if ext == ".mov" || ext == ".mp4" || ext == ".webm" || ext == ".avi" {
			score -= 12000
		}
		if strings.Contains(host, "cdn-icons.flaticon.com") || strings.Contains(host, "flaticon.com") {
			score += 5000
		}
	case "3d":
		switch ext {
		case ".blend":
			score += 9000
		case ".fbx", ".obj", ".zip", ".mtl":
			score += 7000
		case ".png", ".jpg", ".jpeg", ".svg", ".mp4", ".mov", ".webm":
			score -= 12000
		}
	case "regular":
		switch ext {
		case ".zip", ".psd", ".eps", ".ai":
			score += 7000
		case ".jpg", ".jpeg", ".png", ".svg":
			score += 3000
		case ".mov", ".mp4", ".webm", ".avi":
			score -= 2000
		}
	}

	if strings.Contains(strings.ToLower(raw), "filename=") {
		score += 800
	}
	if strings.Contains(strings.ToLower(raw), "token=") {
		score += 400
	}
	return score
}

func responseContextKind(context string) string {
	value := strings.ToLower(strings.TrimSpace(context))
	switch {
	case strings.HasPrefix(value, "video"):
		return "video"
	case strings.HasPrefix(value, "icon"):
		return "icon"
	case strings.HasPrefix(value, "3d"):
		return "3d"
	default:
		return "regular"
	}
}

func extractDirectDownloadURL(resp *http.Response, endpoint string) string {
	if resp == nil || resp.StatusCode >= 400 {
		return ""
	}

	if resolvedURL := finalResponseURL(resp); looksLikeDownloadURL(resolvedURL) {
		return resolvedURL
	}

	if location := strings.TrimSpace(resp.Header.Get("Location")); looksLikeDownloadURL(location) {
		return location
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	contentDisposition := strings.ToLower(resp.Header.Get("Content-Disposition"))
	if !looksLikeDownloadURL(endpoint) {
		return ""
	}

	switch {
	case strings.Contains(contentDisposition, "attachment"):
		return endpoint
	case strings.Contains(contentType, "image/"):
		return endpoint
	case strings.Contains(contentType, "application/zip"):
		return endpoint
	case strings.Contains(contentType, "application/octet-stream"):
		return endpoint
	case strings.Contains(contentType, "binary/octet-stream"):
		return endpoint
	case strings.Contains(contentType, "application/x-zip-compressed"):
		return endpoint
	}

	return ""
}

func finalResponseURL(resp *http.Response) string {
	if resp == nil || resp.Request == nil || resp.Request.URL == nil {
		return ""
	}
	return resp.Request.URL.String()
}

func extractEmbeddedVideoURLs(body []byte) []string {
	text := normalizeEmbeddedText(body)
	matches := anyURLRegex.FindAllString(text, -1)
	values := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		match = strings.TrimSpace(strings.Trim(match, `"'(),;`))
		if !isActualVideoDownloadURL(match) {
			continue
		}
		if _, ok := seen[match]; ok {
			continue
		}
		seen[match] = struct{}{}
		values = append(values, match)
	}
	return values
}

func extractEmbeddedVideoOptionIDs(body []byte) []string {
	text := normalizeEmbeddedText(body)
	matches := make([]string, 0)
	matches = append(matches, findRegexGroupValues(optionIDRegex, text)...)
	matches = append(matches, findRegexGroupValues(optionIDAltRegex, text)...)
	matches = append(matches, findRegexGroupValues(filenameIDRegex, text)...)
	return appendUniqueStrings(nil, matches...)
}

func normalizeEmbeddedText(body []byte) string {
	text := string(body)
	replacer := strings.NewReplacer(`\/`, `/`, `\u0026`, `&`, `\\u0026`, `&`)
	return replacer.Replace(text)
}

func findRegexGroupValues(re *regexp.Regexp, text string) []string {
	if re == nil || text == "" {
		return nil
	}
	found := re.FindAllStringSubmatch(text, -1)
	values := make([]string, 0, len(found))
	for _, item := range found {
		if len(item) < 2 {
			continue
		}
		values = append(values, strings.TrimSpace(item[1]))
	}
	return values
}

func appendUniqueStrings(dst []string, values ...string) []string {
	seen := make(map[string]struct{}, len(dst)+len(values))
	out := make([]string, 0, len(dst)+len(values))
	for _, item := range dst {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	for _, item := range values {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func looksLikeDownloadURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return false
	}

	host := strings.ToLower(parsed.Hostname())
	urlPath := strings.ToLower(parsed.Path)
	if strings.Contains(urlPath, "/api/") {
		return false
	}
	switch {
	case strings.Contains(host, "downloadscdn"):
		return true
	case strings.Contains(host, "videocdn.cdnpk.net"):
		return isActualVideoDownloadPath(urlPath)
	case strings.Contains(host, "gettyimages.com"):
		return strings.Contains(urlPath, "/downloads/")
	case strings.Contains(host, "audiocdn.cdnpk.net"):
		return true
	case strings.Contains(host, "3d.cdnpk.net"):
		return true
	case strings.Contains(host, "flaticon.com") && hasDownloadableExtension(urlPath):
		return true
	case isSupportedAssetHost(host) && strings.Contains(urlPath, "/download/"):
		return true
	}

	return hasDownloadableExtension(urlPath)
}

func isActualVideoDownloadURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return false
	}

	host := strings.ToLower(parsed.Hostname())
	urlPath := strings.ToLower(parsed.Path)
	ext := effectiveDownloadExtension(parsed)

	switch {
	case strings.Contains(host, "videocdn.cdnpk.net"):
		return isActualVideoDownloadPath(urlPath)
	case strings.Contains(host, "downloadscdn"):
		return isDownloadableVideoExtension(ext)
	case strings.Contains(host, "gettyimages.com"):
		return strings.Contains(urlPath, "/downloads/")
	case isSupportedAssetHost(host) && strings.Contains(urlPath, "/download/"):
		return true
	default:
		return false
	}
}

func isSupportedAssetHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	for _, domain := range supportedAssetDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func primaryAssetBaseURL() string {
	raw := strings.TrimSpace(os.Getenv("FREEPIK_SITE_BASE_URL"))
	if raw == "" {
		raw = freepikAuthRefreshURL
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "https://www.magnific.com/"
	}
	parsed.Path = "/"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func primaryAssetHost() string {
	parsed, err := url.Parse(primaryAssetBaseURL())
	if err != nil {
		return "www.magnific.com"
	}
	return parsed.Host
}

func defaultAssetHosts() []string {
	return appendUniqueStrings(nil, primaryAssetHost(), "www.magnific.com", "www.freepik.com")
}

func effectiveDownloadExtension(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	if ext := strings.ToLower(pathpkg.Ext(parsed.Path)); ext != "" {
		return ext
	}
	filename := strings.TrimSpace(parsed.Query().Get("filename"))
	if filename == "" {
		return ""
	}
	return strings.ToLower(pathpkg.Ext(filename))
}

func isDownloadableVideoExtension(ext string) bool {
	switch strings.ToLower(strings.TrimSpace(ext)) {
	case ".mp4", ".mov", ".webm", ".avi", ".zip":
		return true
	default:
		return false
	}
}

func isActualVideoDownloadPath(urlPath string) bool {
	if strings.Contains(urlPath, "/previews/") {
		return false
	}
	if !strings.Contains(urlPath, "/downloads/") {
		return false
	}

	switch {
	case strings.HasSuffix(urlPath, ".mp4"):
		return true
	case strings.HasSuffix(urlPath, ".mov"):
		return true
	case strings.HasSuffix(urlPath, ".webm"):
		return true
	case strings.HasSuffix(urlPath, ".avi"):
		return true
	case strings.HasSuffix(urlPath, ".zip"):
		return true
	default:
		return false
	}
}

func hasDownloadableExtension(urlPath string) bool {
	switch pathpkg.Ext(strings.ToLower(urlPath)) {
	case ".svg", ".png", ".zip", ".eps", ".ai", ".jpg", ".jpeg", ".mp4", ".mov", ".webm", ".avi", ".wav", ".mp3", ".blend", ".obj", ".fbx", ".mtl":
		return true
	default:
		return false
	}
}

func looksLikeSVGURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return false
	}
	return strings.EqualFold(pathpkg.Ext(parsed.Path), ".svg")
}

func isJWTExpired(token string) bool {
	expiresAt, ok := parseJWTExpiry(token)
	if !ok {
		return false
	}

	return !expiresAt.After(time.Now())
}

func parseJWTExpiry(token string) (time.Time, bool) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}

	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, false
	}
	if claims.Exp == 0 {
		return time.Time{}, false
	}

	return time.Unix(claims.Exp, 0), true
}

func getCookieValue(cookieHeader, key string) string {
	for _, chunk := range strings.Split(cookieHeader, ";") {
		part := strings.TrimSpace(chunk)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		if strings.TrimSpace(kv[0]) == key {
			return strings.TrimSpace(kv[1])
		}
	}

	return ""
}
