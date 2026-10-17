// cmd/bilisync/bili.go
//
// B 站接口客户端（仅标准库，零三方依赖）：
//   - 访问首页 + finger/spi 拿 buvid3/buvid4 设备指纹，规避裸请求 412 风控；
//   - nav 接口取 wbi img/sub key，本地生成 mixin key 并对 query 做 wbi 签名；
//   - view 取分 P 列表，player/wbi/v2 取字幕轨，aisubtitle CDN 下载字幕全文，
//     conclusion/get 取官方 AI 总结（摘要 + 带时间戳章节大纲）。
//
// AI 字幕与 AI 总结均要求登录态：SESSDATA 来自环境变量 BILI_SESSDATA（.env，
// 已被 Git 忽略），与 ARK_API_KEY 同级管理，切勿提交仓库。
package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const biliUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// mixinKeyEncTab wbi 签名固定打乱表（官方社区通行版本，64 项）
var mixinKeyEncTab = [64]int{
	46, 47, 18, 2, 53, 8, 23, 32, 15, 50, 10, 31, 58, 3, 45, 35, 27, 43, 5, 49,
	33, 9, 42, 19, 29, 28, 14, 39, 12, 38, 41, 13, 37, 48, 7, 16, 24, 55, 40,
	61, 26, 17, 0, 1, 60, 51, 30, 4, 22, 25, 54, 44, 20, 52, 21, 59, 6, 63,
	57, 62, 11, 36, 20, 34,
}

// BiliClient 带 cookie 罐与 wbi mixin key 缓存的 B 站客户端
type BiliClient struct {
	http      *http.Client
	sessdata  string
	mixinOnce sync.Once
	mixinKey  string
	loginName string
}

func NewBiliClient(sessdata string) *BiliClient {
	jar, _ := cookiejar.New(nil)
	return &BiliClient{
		http:     &http.Client{Jar: jar, Timeout: 25 * time.Second},
		sessdata: strings.TrimSpace(sessdata),
	}
}

// Warmup 建立设备指纹 cookie，必须在业务接口前调用一次
func (c *BiliClient) Warmup() error {
	if _, err := c.get("https://www.bilibili.com/", "https://www.bilibili.com/", false); err != nil {
		return err
	}
	body, err := c.get("https://api.bilibili.com/x/frontend/finger/spi", "https://www.bilibili.com/", false)
	if err != nil {
		return err
	}
	var spi struct {
		Code int `json:"code"`
		Data struct {
			B3 string `json:"b_3"`
			B4 string `json:"b_4"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &spi); err != nil || spi.Code != 0 {
		return fmt.Errorf("获取设备指纹失败: %s", truncate(string(body), 200))
	}
	u, _ := url.Parse("https://api.bilibili.com/")
	c.http.Jar.SetCookies(u, []*http.Cookie{
		{Name: "buvid3", Value: spi.Data.B3, Domain: ".bilibili.com", Path: "/"},
		{Name: "buvid4", Value: spi.Data.B4, Domain: ".bilibili.com", Path: "/"},
	})
	return nil
}

// ---- 业务数据结构 ----

type VideoPage struct {
	CID      int64  `json:"cid"`
	Page     int    `json:"page"`
	Part     string `json:"part"`
	Duration int    `json:"duration"`
}

type VideoInfo struct {
	BVID    string
	AID     int64
	Title   string
	Desc    string
	Owner   string
	UpMid   int64
	PubDate time.Time
	Pages   []VideoPage
}

type SubtitleTrack struct {
	Lan    string `json:"lan"`
	LanDoc string `json:"lan_doc"`
	SubURL string `json:"subtitle_url"`
}

type Cue struct {
	From    float64 `json:"from"`
	To      float64 `json:"to"`
	Content string  `json:"content"`
}

type OutlinePoint struct {
	Timestamp int    `json:"timestamp"`
	Content   string `json:"content"`
}

type OutlineSection struct {
	Timestamp   int            `json:"timestamp"`
	Title       string         `json:"title"`
	PartOutline []OutlinePoint `json:"part_outline"`
}

type Conclusion struct {
	Summary string
	Outline []OutlineSection
}

// View 取视频元信息与分 P 列表
func (c *BiliClient) View(bvid string) (*VideoInfo, error) {
	body, err := c.get("https://api.bilibili.com/x/web-interface/view?bvid="+bvid,
		"https://www.bilibili.com/video/"+bvid, true)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Aid   int64  `json:"aid"`
			Title string `json:"title"`
			Desc  string `json:"desc"`
			Owner struct {
				Name string `json:"name"`
				Mid  int64  `json:"mid"`
			} `json:"owner"`
			Pubdate int64       `json:"pubdate"`
			Pages   []VideoPage `json:"pages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("view 响应解析失败: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("view 接口错误 code=%d msg=%s", resp.Code, resp.Message)
	}
	return &VideoInfo{
		BVID: bvid, AID: resp.Data.Aid, Title: resp.Data.Title, Desc: resp.Data.Desc,
		Owner: resp.Data.Owner.Name, UpMid: resp.Data.Owner.Mid,
		PubDate: time.Unix(resp.Data.Pubdate, 0), Pages: resp.Data.Pages,
	}, nil
}

// SubtitleTracks 取某一 P 的字幕轨；登录态下可见 ai-zh AI 字幕
func (c *BiliClient) SubtitleTracks(bvid string, cid int64) ([]SubtitleTrack, error) {
	signed, err := c.sign("https://api.bilibili.com/x/player/wbi/v2", map[string]string{
		"bvid": bvid,
		"cid":  strconv.FormatInt(cid, 10),
	})
	if err != nil {
		return nil, err
	}
	body, err := c.get(signed, "https://www.bilibili.com/video/"+bvid, true)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Subtitle struct {
				Subtitles []SubtitleTrack `json:"subtitles"`
			} `json:"subtitle"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("player 响应解析失败: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("player 接口错误 code=%d msg=%s", resp.Code, resp.Message)
	}
	return resp.Data.Subtitle.Subtitles, nil
}

// PickSubtitleTrack 按优先级挑中文字幕轨：UP 官字 > AI 中字 > 任意中字
func PickSubtitleTrack(tracks []SubtitleTrack) *SubtitleTrack {
	var aiZH, anyZH *SubtitleTrack
	for i := range tracks {
		t := &tracks[i]
		switch t.Lan {
		case "zh-CN":
			return t // UP 主上传的正式字幕，质量最高
		case "ai-zh":
			aiZH = t
		default:
			if strings.HasPrefix(t.Lan, "zh") && anyZH == nil {
				anyZH = t
			}
		}
	}
	if aiZH != nil {
		return aiZH
	}
	return anyZH
}

// DownloadSubtitle 下载字幕 JSON（CDN 地址为协议相对 URL，无需 cookie，但要带 Referer）
func (c *BiliClient) DownloadSubtitle(subtitleURL string) ([]Cue, error) {
	u := subtitleURL
	if strings.HasPrefix(u, "//") {
		u = "https:" + u
	}
	body, err := c.get(u, "https://www.bilibili.com/", false)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Body []Cue `json:"body"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("字幕 JSON 解析失败（可能鉴权链接已过期，请重试）: %w", err)
	}
	return resp.Body, nil
}

// Conclusion 取官方 AI 总结；该接口并非每个视频都有，失败返回 nil, nil 由调用方降级
func (c *BiliClient) Conclusion(bvid string, cid, upMid int64) (*Conclusion, error) {
	signed, err := c.sign("https://api.bilibili.com/x/web-interface/view/conclusion/get", map[string]string{
		"bvid":   bvid,
		"cid":    strconv.FormatInt(cid, 10),
		"up_mid": strconv.FormatInt(upMid, 10),
	})
	if err != nil {
		return nil, err
	}
	body, err := c.get(signed, "https://www.bilibili.com/video/"+bvid, true)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			ModelResult struct {
				Summary string           `json:"summary"`
				Outline []OutlineSection `json:"outline"`
			} `json:"model_result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, nil // 响应异常一律按"无大纲"降级，不阻断字幕主流程
	}
	if resp.Code != 0 || len(resp.Data.ModelResult.Outline) == 0 {
		return nil, nil
	}
	return &Conclusion{
		Summary: strings.TrimSpace(resp.Data.ModelResult.Summary),
		Outline: resp.Data.ModelResult.Outline,
	}, nil
}

// ---- 内部实现 ----

var keyURLHashRe = regexp.MustCompile(`/([0-9a-f]{32})\.\w+`)

// ensureMixinKey 懒加载 wbi mixin key；同时校验登录态（未登录 nav 也返回 key，
// 但 AI 字幕/总结需要登录，这里顺手取出用户名用于日志）
func (c *BiliClient) ensureMixinKey() error {
	var err error
	c.mixinOnce.Do(func() {
		body, e := c.get("https://api.bilibili.com/x/web-interface/nav", "https://www.bilibili.com/", true)
		if e != nil {
			err = e
			return
		}
		var nav struct {
			Code int `json:"code"`
			Data struct {
				IsLogin bool   `json:"isLogin"`
				Uname   string `json:"uname"`
				WbiImg  struct {
					ImgURL string `json:"img_url"`
					SubURL string `json:"sub_url"`
				} `json:"wbi_img"`
			} `json:"data"`
		}
		if e := json.Unmarshal(body, &nav); e != nil {
			err = fmt.Errorf("nav 响应解析失败: %w", e)
			return
		}
		if !nav.Data.IsLogin {
			err = errors.New("BILI_SESSDATA 无效或已过期（nav 显示未登录），请重新从浏览器获取")
			return
		}
		m1 := keyURLHashRe.FindStringSubmatch(nav.Data.WbiImg.ImgURL)
		m2 := keyURLHashRe.FindStringSubmatch(nav.Data.WbiImg.SubURL)
		if m1 == nil || m2 == nil {
			err = errors.New("nav 未返回 wbi key，接口结构可能已变更")
			return
		}
		raw := m1[1] + m2[1]
		var b strings.Builder
		for _, idx := range mixinKeyEncTab {
			b.WriteByte(raw[idx])
		}
		// 官方算法：64 位原始 key 按打乱表重排后只取前 32 位
		c.mixinKey = b.String()[:32]
		c.loginName = nav.Data.Uname
	})
	return err
}

// sign 生成 wbi 签名 URL（自动追加 wts 与 w_rid）
func (c *BiliClient) sign(base string, params map[string]string) (string, error) {
	if err := c.ensureMixinKey(); err != nil {
		return "", err
	}
	params["wts"] = strconv.FormatInt(time.Now().Unix(), 10)
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// wbi 要求过滤值中的 !'()* 特殊字符
	filtered := regexp.MustCompile(`[!'()*]`)
	var q strings.Builder
	for i, k := range keys {
		if i > 0 {
			q.WriteByte('&')
		}
		q.WriteString(url.QueryEscape(k))
		q.WriteByte('=')
		q.WriteString(url.QueryEscape(filtered.ReplaceAllString(params[k], "")))
	}
	sum := md5.Sum([]byte(q.String() + c.mixinKey))
	return base + "?" + q.String() + "&w_rid=" + hex.EncodeToString(sum[:]), nil
}

// get 统一 HTTP GET：浏览器 UA、Referer、可选 SESSDATA
func (c *BiliClient) get(u, referer string, withAuth bool) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", biliUA)
	req.Header.Set("Referer", referer)
	if withAuth && c.sessdata != "" {
		req.AddCookie(&http.Cookie{Name: "SESSDATA", Value: c.sessdata})
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return body, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
