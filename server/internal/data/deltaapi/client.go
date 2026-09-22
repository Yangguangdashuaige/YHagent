// internal/data/deltaapi/client.go
//
// 三角洲数据帝开放平台 HTTP 客户端。
// 文档来源：server/docs/delta_api1.md / delta_api2.md / delta_api3.md
// 服务地址：https://orzice.com/workApi
// 鉴权方式：所有接口统一通过 query 参数 token 传递密钥。
//
// 设计原则：
//  1. 每个文档接口对应一个强类型入参结构 + 一个 Client 方法，返回原始 JSON（data 字段），
//     因为消费方是大模型，保留完整字段最利于模型理解；
//  2. 公共的 token 注入、超时、业务码校验、缓存复用全部收敛在 doGet，新增接口只加方法；
//  3. 不依赖任何 CGO 三方库，仅用标准库 net/http，契合单 EXE / 低内存目标。
package deltaapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"yhagent-server/internal/data/cache"
)

// DefaultBaseURL 三角洲数据帝正式环境地址（DELTA_API_BASE 未配置时使用）
const DefaultBaseURL = "https://orzice.com/workApi"

// 接口路径常量，新增接口时在这里集中登记
const (
	pathItemInfoAll       = "/v1/sjz_api/item_info_all"     // 【全物品】基础信息库
	pathItemPriceAll      = "/v1/sjz_api/item_price_all"    // 【全物品】最新价格数据（每周战备）
	pathFakeAccountTop    = "/v1/sjz_api/jzv3_top"          // 假账排行榜
	pathReadyBuildV4      = "/v1/sjz_api/jzv4_zb"           // 实时卡战备V4
	pathItemListPro       = "/v1/sjz_api/item_list_pro"     // 全物品列表 Pro
	pathItemListProKey    = "/v1/sjz_api/item_list_pro_key" // 全物品列表 Pro - 筛选项
	pathItemList          = "/v1/sjz_api/item_list"         // 全物品列表/Top排行（分页）
	pathDIYReadyBuild     = "/v1/sjz_api/jzv3_diy"          // DIY卡战备 - 自定义凑战备
	pathReadyBuildV3      = "/v1/sjz_api/jzv3_zb"           // 实时卡战备V3 - 市场
	pathReadyBuildV3All   = "/v1/sjz_api/jzv3_zb_all"       // 实时卡战备V3 - 市场 - ALL
	pathReadyBuildPlus    = "/v1/sjz_api/jzv3_zb_plus"      // 实时卡战备V3 - 军需处兑换+市场
	pathReadyBuildPlusAll = "/v1/sjz_api/jzv3_zb_plus_all"  // 实时卡战备V3 - 军需处兑换+市场 - ALL
	pathManufacturePro    = "/v1/sjz_api/manufacturePro"    // 特勤处制造利润Pro
	pathExchange          = "/v1/sjz_api/exchange"          // 军需处兑换 - 成本
	pathExchangePay       = "/v1/sjz_api/exchange_pay"      // 军需处兑换 - 子弹收益
	pathExchangeZb        = "/v1/sjz_api/exchange_zb"       // 军需处兑换 - 战备假账
	pathKeysDay           = "/v1/sjz_api/keys_day"          // 今日钥匙卡低价预测
	pathAmmoPack          = "/v1/sjz_api/ammo_pack"         // 子弹自选包收益
	pathAmmoDay           = "/v1/sjz_api/ammo_day"          // 今日倒子弹低价预测
	pathGunBuildCode      = "/v1/sjz_api/gun_gqm"           // 改枪码 - 烽火
	pathSeasonTaskS11     = "/v1/sjz_api/task_s11"          // 赛季任务 - S11
	pathMapPassword       = "/v1/sjz_api/map_pwd"           // 今日密码
	pathMarket            = "/v1/sjz_api/market"            // 当前市集所需物品
)

// 各类数据的缓存时长严格按上游文档标注的更新频率分级：
// 更新越慢的数据缓存越久（省资源点），每分钟更新的行情只缓存 60 秒。
const (
	cacheStaticTTL   = 24 * time.Hour // 基础信息库/筛选项：赛季内近乎静态（单次20点资源）
	cacheSeasonTTL   = 6 * time.Hour  // 赛季任务：随赛季更新（单次10点资源）
	cacheBuildTTL    = 6 * time.Hour  // 改枪码：社区方案沉淀，变化极慢
	cacheDailyTTL    = 2 * time.Hour  // 文档标注"每天0点更新"：钥匙卡预测、倒子弹
	cachePasswordTTL = 30 * time.Minute
	cacheMarketTTL   = 30 * time.Minute // 市集所需物品：随轮换变化
	cacheLongTTL     = 10 * time.Minute // 文档标注"10分钟更新"：排行榜、制造利润、兑换成本
	cacheNormalTTL   = 10 * time.Minute // 弹药包等普通分析类，与上游更新节奏对齐
	cacheWeeklyTTL   = 10 * time.Minute // 全物品价格快照（20点资源/次），平衡时效与点数消耗
	cachePriceTTL    = 60 * time.Second // 文档标注"每分钟更新"：实时价格/排行、卡战备
	cacheDIYTTL      = 3 * time.Minute  // DIY卡战备（单次10点资源），结果时效性强
)

// httpTimeout 三角洲接口整体超时时间（DIY计算文档说明一般3秒内出结果，留足余量）
const httpTimeout = 25 * time.Second

// Client 三角洲开放平台客户端，零 CGO 依赖，可安全并发使用
type Client struct {
	apiKey   string
	baseURL  string
	http     *http.Client
	memCache *cache.Cache // 可选，为空则不缓存
	flights  flightGroup  // 缓存击穿保护：同 key 并发请求合并
}

// NewClient 创建客户端：apiKey 对应 .env 的 DELTA_API_KEY，baseURL 为空时走正式环境
func NewClient(apiKey, baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		http:    &http.Client{Timeout: httpTimeout},
	}
}

// WithCache 挂载项目自带的内存缓存，返回客户端自身便于链式初始化
func (c *Client) WithCache(ca *cache.Cache) *Client {
	c.memCache = ca
	return c
}

// WithHTTPClient 替换底层 http.Client（主要留给测试用）
func (c *Client) WithHTTPClient(hc *http.Client) *Client {
	if hc != nil {
		c.http = hc
	}
	return c
}

// envelope 三角洲接口统一响应外壳（部分接口 msg/message 两个字段并存）
type envelope struct {
	Code    int             `json:"code"`
	Msg     string          `json:"msg"`
	Message string          `json:"message"`
	Count   int             `json:"count"`
	Data    json.RawMessage `json:"data"`
}

// doGet 执行一次接口请求：拼参数 → 查缓存 → 发请求 → 校验业务码 → 写缓存。
// query 只放业务参数，token 由本方法统一注入且不参与缓存键。
// ttl <= 0 表示不缓存。
func (c *Client) doGet(ctx context.Context, apiPath string, query url.Values, ttl time.Duration) (json.RawMessage, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("三角洲API密钥未配置，请在 .env 中设置 DELTA_API_KEY")
	}

	cacheKey := apiPath
	if len(query) > 0 {
		cacheKey += "?" + query.Encode()
	}
	if ttl > 0 && c.memCache != nil {
		if cached, ok := c.memCache.Get(cacheKey); ok {
			if raw, ok := cached.(json.RawMessage); ok {
				return raw, nil
			}
		}
	}

	// 缓存未命中：singleflight 合并同 key 并发调用，上游三角洲接口同一时刻只会真正请求一次
	return c.flights.Do(cacheKey, func() (json.RawMessage, error) {
		// 等待首个调用期间缓存可能已被填充，进入临界区后再查一次
		if ttl > 0 && c.memCache != nil {
			if cached, ok := c.memCache.Get(cacheKey); ok {
				if raw, ok := cached.(json.RawMessage); ok {
					return raw, nil
				}
			}
		}

		// 统一注入密钥
		query.Set("token", c.apiKey)

		reqURL := c.baseURL + apiPath + "?" + query.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("构造请求失败: %w", err)
		}
		req.Header.Set("Accept", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("请求三角洲接口失败: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20)) // 单接口响应上限 20MB，防异常响应吃光内存
		if err != nil {
			return nil, fmt.Errorf("读取响应失败: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("三角洲接口HTTP状态异常: %d", resp.StatusCode)
		}

		var env envelope
		if err := json.Unmarshal(body, &env); err != nil {
			return nil, fmt.Errorf("解析响应失败: %w", err)
		}
		// 上游各接口成功码不统一：绝大多数为 0，改枪码等部分接口为 200，两者都视为成功
		if env.Code != 0 && env.Code != 200 {
			tip := env.Msg
			if tip == "" {
				tip = env.Message
			}
			if tip == "" {
				tip = fmt.Sprintf("业务码 %d", env.Code)
			}
			return nil, fmt.Errorf("三角洲接口返回错误: %s", tip)
		}

		if ttl > 0 && c.memCache != nil && len(env.Data) > 0 {
			dataCopy := make(json.RawMessage, len(env.Data))
			copy(dataCopy, env.Data)
			c.memCache.Set(cacheKey, dataCopy, ttl)
		}
		return env.Data, nil
	})
}

// ---- 查询参数小工具：三角洲接口未传的可选参数应当缺省，而不是统一传 0 ----

func queryValues() url.Values {
	return url.Values{}
}

func setInt(q url.Values, key string, value int) {
	if value != 0 {
		q.Set(key, fmt.Sprintf("%d", value))
	}
}

// setPositiveIntWithDefault 必填整型参数缺省时使用默认值
func setPositiveIntWithDefault(q url.Values, key string, value, def int) {
	if value <= 0 {
		value = def
	}
	q.Set(key, fmt.Sprintf("%d", value))
}

func setString(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}

// setStringWithDefault 必填字符串参数缺省时使用默认值
func setStringWithDefault(q url.Values, key, value, def string) {
	if value == "" {
		value = def
	}
	q.Set(key, value)
}

// ============================================================================
// delta_api1.md —— 基础数据 / 实时交易行 / 卡战备
// ============================================================================

// AllItemInfo 【全物品】基础信息库（2000+物品，单次消耗20点资源，谨慎调用）
func (c *Client) AllItemInfo(ctx context.Context) (json.RawMessage, error) {
	return c.doGet(ctx, pathItemInfoAll, queryValues(), cacheStaticTTL)
}

// ItemPriceAllParams 全物品最新价格入参
type ItemPriceAllParams struct {
	IsZb int `json:"isZb"` // 0普通数据 1战备数据（战备暂不开放）
}

// AllItemPrice 【全物品】最新价格数据（每周战备数据，单次消耗20点资源，谨慎调用）
func (c *Client) AllItemPrice(ctx context.Context, p ItemPriceAllParams) (json.RawMessage, error) {
	q := queryValues()
	setInt(q, "isZb", p.IsZb)
	return c.doGet(ctx, pathItemPriceAll, q, cacheWeeklyTTL)
}

// FakeAccountRankParams 假账排行榜入参
type FakeAccountRankParams struct {
	Category string `json:"a"` // 类型：all/acc配件/helmet头/armor甲/chest胸挂/bag背包/gun枪械
}

// FakeAccountRank 假账排行榜（10分钟更新一次，消耗2点资源）
func (c *Client) FakeAccountRank(ctx context.Context, p FakeAccountRankParams) (json.RawMessage, error) {
	q := queryValues()
	setStringWithDefault(q, "a", p.Category, "all")
	return c.doGet(ctx, pathFakeAccountTop, q, cacheLongTTL)
}

// ReadyBuildParams 卡战备档位入参（V3/V4 共用）
type ReadyBuildParams struct {
	Lv int `json:"lv"` // 0:11W机密 1:18W机密 2:55W绝密巴克什 3:60W绝密航天 5:78W绝密监狱
}

// ReadyBuildV4 实时卡战备V4 - 多种配装逻辑（每分钟更新，消耗1点资源）
func (c *Client) ReadyBuildV4(ctx context.Context, p ReadyBuildParams) (json.RawMessage, error) {
	q := queryValues()
	setInt(q, "lv", p.Lv)
	return c.doGet(ctx, pathReadyBuildV4, q, cachePriceTTL)
}

// ItemListProParams 全物品列表 Pro 入参
type ItemListProParams struct {
	Page  int    `json:"p"`      // 页码，默认1
	Limit int    `json:"limit"`  // 每页数量 10~20，默认10
	Name  string `json:"n"`      // 名字模糊搜索（40字符以内）
	Key1  string `json:"key1"`   // 分类索引1（必填，先调 FilterKeyOptions 获取）
	Key2  string `json:"key2"`   // 分类索引2（必填）
	Sort  string `json:"sort"`   // 排序，如 1-1/1-2/2-1/2-2/3-1/3-2/5-1/7-1/30-1
	Grade int    `json:"grade"`  // 等级筛选 0~6
	IsPay int    `json:"is_pay"` // 1=必须可在交易行上架
}

// ItemListPro 全物品列表 Pro（消耗1点资源，推荐的物品价格/排行查询入口）
func (c *Client) ItemListPro(ctx context.Context, p ItemListProParams) (json.RawMessage, error) {
	q := queryValues()
	setPositiveIntWithDefault(q, "p", p.Page, 1)
	setPositiveIntWithDefault(q, "limit", p.Limit, 10)
	setString(q, "n", p.Name)
	setString(q, "key1", p.Key1)
	setString(q, "key2", p.Key2)
	setStringWithDefault(q, "sort", p.Sort, "1-1")
	setInt(q, "grade", p.Grade)
	setInt(q, "is_pay", p.IsPay)
	return c.doGet(ctx, pathItemListPro, q, cachePriceTTL)
}

// FilterKeyOptions 全物品列表 Pro - 筛选项（不消耗资源，建议只请求一次，已做长缓存）
func (c *Client) FilterKeyOptions(ctx context.Context) (json.RawMessage, error) {
	return c.doGet(ctx, pathItemListProKey, queryValues(), cacheStaticTTL)
}

// ItemListParams 全物品列表/Top排行 入参
type ItemListParams struct {
	Lx    string `json:"lx"`    // list 列表 / top 排名，默认 list
	Page  int    `json:"p"`     // 页码（top模式不支持）
	Limit int    `json:"limit"` // 每页 10~20；top模式只支持 3 或 5
	Types string `json:"types"` // 数据类型：zhanbei/acc/ammo/gun/collection/consume/helmet/armor/bag/chest/key
	Top   string `json:"top"`   // 排序，如 1-1/2-2/80-1/81-1
	Name  string `json:"n"`     // 模糊搜索（top模式不支持）
	Mid   int    `json:"mid"`   // 地图筛选 1大坝 2航天 3长弓 4巴克什 5监狱（一般配钥匙卡）
	Grade int    `json:"grade"` // 等级 0~6
	Mtype int    `json:"mtype"` // 收集品类型 1工艺藏品 2工具材料 3电子物品 4家居物品 5能源燃料 6医疗道具 7资料情报
}

// ItemList 全物品列表/Top排行（分页），战备类2点资源、非战备1点资源
func (c *Client) ItemList(ctx context.Context, p ItemListParams) (json.RawMessage, error) {
	q := queryValues()
	setStringWithDefault(q, "lx", p.Lx, "list")
	setPositiveIntWithDefault(q, "p", p.Page, 1)
	setPositiveIntWithDefault(q, "limit", p.Limit, 10)
	setString(q, "types", p.Types)
	setStringWithDefault(q, "top", p.Top, "1-1")
	setString(q, "n", p.Name)
	setInt(q, "mid", p.Mid)
	setInt(q, "grade", p.Grade)
	setInt(q, "mtype", p.Mtype)
	return c.doGet(ctx, pathItemList, q, cachePriceTTL)
}

// ============================================================================
// delta_api2.md —— 鼠鼠卡战备系列
// ============================================================================

// DIYReadyBuildParams DIY卡战备自定义凑战备入参（所有开关 0禁止 1启用）
type DIYReadyBuildParams struct {
	Zb        int `json:"zb"`          // 目标战备价值，1500~780000
	Exchange  int `json:"exchange"`    // 是否使用部门兑换
	IsBb      int `json:"is_bb"`       // 背包
	IsGun     int `json:"is_gun"`      // 枪械和配件
	IsGun2Off int `json:"is_gun2_off"` // 关闭双枪
	IsHj      int `json:"is_hj"`       // 护甲
	IsSq      int `json:"is_sq"`       // 手枪
	IsTk      int `json:"is_tk"`       // 头盔
	IsXg      int `json:"is_xg"`       // 胸挂
}

// DIYReadyBuild DIY卡战备 - 自定义凑战备（高成本计算服务，单次消耗10点资源，约3秒出结果）
func (c *Client) DIYReadyBuild(ctx context.Context, p DIYReadyBuildParams) (json.RawMessage, error) {
	q := queryValues()
	setInt(q, "zb", p.Zb)
	setInt(q, "exchange", p.Exchange)
	setInt(q, "is_bb", p.IsBb)
	setInt(q, "is_gun", p.IsGun)
	setInt(q, "is_gun2_off", p.IsGun2Off)
	setInt(q, "is_hj", p.IsHj)
	setInt(q, "is_sq", p.IsSq)
	setInt(q, "is_tk", p.IsTk)
	setInt(q, "is_xg", p.IsXg)
	return c.doGet(ctx, pathDIYReadyBuild, q, cacheDIYTTL)
}

// ReadyBuildV3 实时卡战备V3 - 市场
func (c *Client) ReadyBuildV3(ctx context.Context, p ReadyBuildParams) (json.RawMessage, error) {
	q := queryValues()
	setInt(q, "lv", p.Lv)
	return c.doGet(ctx, pathReadyBuildV3, q, cachePriceTTL)
}

// ReadyBuildV3All 实时卡战备V3 - 市场 - ALL（全档位数据）
func (c *Client) ReadyBuildV3All(ctx context.Context) (json.RawMessage, error) {
	return c.doGet(ctx, pathReadyBuildV3All, queryValues(), cachePriceTTL)
}

// ReadyBuildV3Plus 实时卡战备V3 - 军需处兑换+市场
func (c *Client) ReadyBuildV3Plus(ctx context.Context, p ReadyBuildParams) (json.RawMessage, error) {
	q := queryValues()
	setInt(q, "lv", p.Lv)
	return c.doGet(ctx, pathReadyBuildPlus, q, cachePriceTTL)
}

// ReadyBuildV3PlusAll 实时卡战备V3 - 军需处兑换+市场 - ALL（全档位数据）
func (c *Client) ReadyBuildV3PlusAll(ctx context.Context) (json.RawMessage, error) {
	return c.doGet(ctx, pathReadyBuildPlusAll, queryValues(), cachePriceTTL)
}

// ============================================================================
// delta_api3.md —— 制造/兑换/钥匙子弹分析/改枪码/任务/密码/市集
// ============================================================================

// ManufactureParams 特勤处制造利润入参
type ManufactureParams struct {
	Type      int `json:"t"` // 1技术中心 2工作台 3制药台 4防具台
	DeptLevel int `json:"l"` // 部门等级
}

// ManufacturePro 特勤处制造利润Pro - 自有源计算（文档标注10分钟更新，消耗1点资源）
func (c *Client) ManufacturePro(ctx context.Context, p ManufactureParams) (json.RawMessage, error) {
	q := queryValues()
	setInt(q, "t", p.Type)
	setInt(q, "l", p.DeptLevel)
	return c.doGet(ctx, pathManufacturePro, q, cacheLongTTL)
}

// Exchange 军需处兑换 - 成本，deptType：1战斗 2医疗 3后勤 4战术 5研发
func (c *Client) Exchange(ctx context.Context, deptType int) (json.RawMessage, error) {
	q := queryValues()
	setInt(q, "t", deptType)
	return c.doGet(ctx, pathExchange, q, cacheLongTTL)
}

// ExchangePay 军需处兑换 - 子弹收益（10分钟更新，消耗1点资源）
func (c *Client) ExchangePay(ctx context.Context) (json.RawMessage, error) {
	return c.doGet(ctx, pathExchangePay, queryValues(), cacheLongTTL)
}

// ExchangeZb 军需处兑换 - 战备假账（10分钟更新，消耗1点资源）
func (c *Client) ExchangeZb(ctx context.Context) (json.RawMessage, error) {
	return c.doGet(ctx, pathExchangeZb, queryValues(), cacheLongTTL)
}

// KeysDay 今日钥匙卡低价预测，mapID：1大坝 2航天 3长弓 4巴克什 5监狱（文档标注每天0点更新，消耗1点资源）
func (c *Client) KeysDay(ctx context.Context, mapID int) (json.RawMessage, error) {
	q := queryValues()
	setPositiveIntWithDefault(q, "mid", mapID, 1)
	return c.doGet(ctx, pathKeysDay, q, cacheDailyTTL)
}

// AmmoPack 子弹自选包收益，grade 为品质等级 0~6（消耗1点资源）
func (c *Client) AmmoPack(ctx context.Context, grade int) (json.RawMessage, error) {
	q := queryValues()
	setPositiveIntWithDefault(q, "grade", grade, 0)
	return c.doGet(ctx, pathAmmoPack, q, cacheNormalTTL)
}

// AmmoDay 今日倒子弹低价预测，grade 为品质等级 0~6（文档标注每天0点更新）
func (c *Client) AmmoDay(ctx context.Context, grade int) (json.RawMessage, error) {
	q := queryValues()
	setPositiveIntWithDefault(q, "grade", grade, 0)
	return c.doGet(ctx, pathAmmoDay, q, cacheDailyTTL)
}

// GunBuildCodeParams 改枪码查询入参
type GunBuildCodeParams struct {
	Page  int    `json:"p"`     // 页码
	Mtype string `json:"mtype"` // 枪械分类
	Gun   string `json:"gun"`   // 枪械名称
}

// GunBuildCode 改枪码 - 烽火（消耗1点资源）。社区方案沉淀变化极慢，长缓存 6 小时。
func (c *Client) GunBuildCode(ctx context.Context, p GunBuildCodeParams) (json.RawMessage, error) {
	q := queryValues()
	setPositiveIntWithDefault(q, "p", p.Page, 1)
	setString(q, "mtype", p.Mtype)
	setString(q, "gun", p.Gun)
	return c.doGet(ctx, pathGunBuildCode, q, cacheBuildTTL)
}

// SeasonTaskS11 赛季任务 - S11（单次消耗10点资源，随赛季更新）
func (c *Client) SeasonTaskS11(ctx context.Context) (json.RawMessage, error) {
	return c.doGet(ctx, pathSeasonTaskS11, queryValues(), cacheSeasonTTL)
}

// MapPassword 今日密码（消耗1点资源）
func (c *Client) MapPassword(ctx context.Context) (json.RawMessage, error) {
	return c.doGet(ctx, pathMapPassword, queryValues(), cachePasswordTTL)
}

// Market 当前市集所需物品（消耗1点资源）
func (c *Client) Market(ctx context.Context) (json.RawMessage, error) {
	return c.doGet(ctx, pathMarket, queryValues(), cacheMarketTTL)
}
