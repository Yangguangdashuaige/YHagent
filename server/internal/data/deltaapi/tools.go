// internal/data/deltaapi/tools.go
//
// 三角洲全部接口的豆包 Function Call 工具封装。
// 新增接口接入步骤（只注册，不改 Agent 核心）：
//  1. 在 client.go 增加一个 Client 方法（对应文档路径和参数）；
//  2. 在本文件 RegisterTools 里 reg.Register(ai.NewFunctionTool(...)) 一行注册；
//  3. 完成。Agent 主循环、会话、错误降级都无需改动。
package deltaapi

import (
	"context"
	"encoding/json"
	"fmt"

	"yhagent-server/internal/ai"
)

// RegisterTools 把三角洲数据帝文档（delta_api1~3.md）里的全部 23 个接口注册为 Agent 工具
func RegisterTools(reg *ai.Registry, c *Client) {
	// ---------------- delta_api1.md：基础数据 / 交易行 / 卡战备 ----------------

	// 1. 全物品基础信息库（高成本，强力劝退模型随意调用）
	reg.Register(ai.NewFunctionTool(
		"delta_all_item_info",
		"【慎用·单次消耗20点资源】获取全部2000+物品的基础档案（名称、分类、等级、占格、介绍等）。"+
			"仅当用户明确要求导出/遍历全物品档案时使用；查某个物品价格请改用 delta_item_list 或 delta_item_list_pro。",
		map[string]any{},
		nil,
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			data, err := c.AllItemInfo(ctx)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 10), nil
		},
	))

	// 2. 全物品最新价格
	reg.Register(ai.NewFunctionTool(
		"delta_all_item_price",
		"【慎用·单次消耗20点资源】获取全部物品最新价格周数据（现价、今日起价、3/7/30日价格与涨跌）。"+
			"查单个/少量物品价格不要用它，改用 delta_item_list 分页模糊搜索。",
		map[string]any{
			"isZb": ai.IntProp("0=普通数据（默认），1=战备数据（暂不开放）"),
		},
		nil,
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := decodeArgs[ItemPriceAllParams](raw)
			if err != nil {
				return nil, err
			}
			data, err := c.AllItemPrice(ctx, p)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 10), nil
		},
	))

	// 3. 假账排行榜
	reg.Register(ai.NewFunctionTool(
		"delta_fake_account_rank",
		"查询各类装备的假账（战备值与市价差值）排行榜，10分钟更新。可用于找差价最大的装备。",
		map[string]any{
			"a": ai.EnumProp("装备类别，默认 all 全部", "all", "acc", "helmet", "armor", "chest", "bag", "gun"),
		},
		nil,
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := decodeArgs[FakeAccountRankParams](raw)
			if err != nil {
				return nil, err
			}
			data, err := c.FakeAccountRank(ctx, p)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 20), nil
		},
	))

	// 4. 实时卡战备V4
	reg.Register(ai.NewFunctionTool(
		"delta_ready_build_v4",
		"查询实时卡战备V4配装方案（市场购买为主，含花费最少/实用优先等多套方案），每分钟更新。"+
			"用户说“卡战备、凑战备、11W/18W/55W/60W/78W配装”时优先使用。",
		map[string]any{
			"lv": ai.EnumIntProp("战备档位：0=11W机密 1=18W机密 2=55W绝密巴克什 3=60W绝密航天 5=78W绝密监狱", 0, 1, 2, 3, 5),
		},
		nil,
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := decodeArgs[ReadyBuildParams](raw)
			if err != nil {
				return nil, err
			}
			data, err := c.ReadyBuildV4(ctx, p)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 20), nil
		},
	))

	// 5. 全物品列表 Pro
	reg.Register(ai.NewFunctionTool(
		"delta_item_list_pro",
		"按分类分页查询交易行物品列表Pro（含现价、3/7/30日价、涨跌、推荐出售方式、占格等），支持名称模糊搜索与多维排序。"+
			"key1/key2 必须先调用 delta_item_filter_keys 获取合法分类索引。",
		map[string]any{
			"key1":   ai.StrProp("分类索引1（必填），通过 delta_item_filter_keys 获取，例如 钥匙"),
			"key2":   ai.StrProp("分类索引2（必填），通过 delta_item_filter_keys 获取，例如 零号大坝"),
			"p":      ai.IntProp("页码，从1开始，默认1"),
			"limit":  ai.IntProp("每页数量10~20，默认10"),
			"n":      ai.StrProp("物品名称模糊搜索，40字符以内，区分大小写"),
			"sort":   ai.EnumProp("排序方式：1-1今日涨跌升 1-2降 2-1金额升 2-2金额降 3-1价值升 3-2价值降 5-x为3日 7-x为7日 30-x为30日", "1-1", "1-2", "2-1", "2-2", "3-1", "3-2", "5-1", "5-2", "7-1", "7-2", "30-1", "30-2"),
			"grade":  ai.EnumIntProp("品质等级筛选 0~6", 0, 1, 2, 3, 4, 5, 6),
			"is_pay": ai.IntProp("传1表示只看可在交易行上架的物品"),
		},
		[]string{"key1", "key2"},
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := decodeArgs[ItemListProParams](raw)
			if err != nil {
				return nil, err
			}
			data, err := c.ItemListPro(ctx, p)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 20), nil
		},
	))

	// 6. 全物品列表 Pro 筛选项（免费）
	reg.Register(ai.NewFunctionTool(
		"delta_item_filter_keys",
		"获取 delta_item_list_pro 所需的分类筛选项（key1/key2 索引及中文标签）。不消耗资源。"+
			"在使用 delta_item_list_pro 前若不确定分类索引，应先调用本工具。",
		map[string]any{},
		nil,
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			return c.FilterKeyOptions(ctx)
		},
	))

	// 7. 全物品列表/Top排行
	reg.Register(ai.NewFunctionTool(
		"delta_item_list",
		"按类型分页查询全物品列表或Top排行榜（含实时价格/涨跌/假账），支持名称模糊搜索。"+
			"用户报物品名字问价、问某类物品排行时最常用。查钥匙卡可配合 mid 地图筛选。",
		map[string]any{
			"types": ai.EnumProp("物品类型（必填）", "zhanbei", "acc", "ammo", "gun", "collection", "consume", "helmet", "armor", "bag", "chest", "key"),
			"lx":    ai.EnumProp("查询模式：list=分页列表（默认） top=排行榜（不支持n和p，limit只支持3或5）", "list", "top"),
			"p":     ai.IntProp("页码，默认1（top模式无效）"),
			"limit": ai.IntProp("每页数量10~20，默认10；top模式仅支持3或5"),
			"top":   ai.StrProp("排序：1-x今日涨跌 2-x金额 3-x价值 5-x三日 7-x七日 30-x三十日 80-x假账 81-x假账比例，x=1升序2降序"),
			"n":     ai.StrProp("物品名称模糊搜索（top模式无效）"),
			"mid":   ai.EnumIntProp("地图筛选（常配钥匙卡）：1大坝 2航天 3长弓 4巴克什 5潮汐监狱", 1, 2, 3, 4, 5),
			"grade": ai.EnumIntProp("品质等级 0~6", 0, 1, 2, 3, 4, 5, 6),
			"mtype": ai.EnumIntProp("收集品小类（仅 types=collection 时有效）：1工艺藏品 2工具材料 3电子物品 4家居物品 5能源燃料 6医疗道具 7资料情报", 1, 2, 3, 4, 5, 6, 7),
		},
		[]string{"types"},
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := decodeArgs[ItemListParams](raw)
			if err != nil {
				return nil, err
			}
			data, err := c.ItemList(ctx, p)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 20), nil
		},
	))

	// ---------------- delta_api2.md：DIY / V3 卡战备系列 ----------------

	// 8. DIY卡战备
	reg.Register(ai.NewFunctionTool(
		"delta_diy_ready_build",
		"【单次消耗10点资源】DIY自定义凑战备：给定目标战备值和允许使用的部位开关，返回最省钱的配装组合，约3秒出结果。",
		map[string]any{
			"zb":          ai.IntProp("目标战备价值（必填），范围1500~780000"),
			"exchange":    ai.IntProp("是否允许军需处兑换：0禁止（默认） 1启用"),
			"is_gun":      ai.IntProp("枪械和配件：0禁止 1启用"),
			"is_gun2_off": ai.IntProp("关闭双枪：0否 1是"),
			"is_tk":       ai.IntProp("头盔：0禁止 1启用"),
			"is_hj":       ai.IntProp("护甲：0禁止 1启用"),
			"is_xg":       ai.IntProp("胸挂：0禁止 1启用"),
			"is_bb":       ai.IntProp("背包：0禁止 1启用"),
			"is_sq":       ai.IntProp("手枪：0禁止 1启用"),
		},
		[]string{"zb"},
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := decodeArgs[DIYReadyBuildParams](raw)
			if err != nil {
				return nil, err
			}
			data, err := c.DIYReadyBuild(ctx, p)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 20), nil
		},
	))

	// 9. 实时卡战备V3 - 市场
	reg.Register(ai.NewFunctionTool(
		"delta_ready_build_v3",
		"查询实时卡战备V3市场方案（仅交易行购买），按档位返回配装。",
		map[string]any{
			"lv": ai.EnumIntProp("战备档位：0=11W 1=18W 2=55W巴克什 3=60W航天 5=78W监狱", 0, 1, 2, 3, 5),
		},
		nil,
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := decodeArgs[ReadyBuildParams](raw)
			if err != nil {
				return nil, err
			}
			data, err := c.ReadyBuildV3(ctx, p)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 20), nil
		},
	))

	// 10. 实时卡战备V3 - 市场 - ALL
	reg.Register(ai.NewFunctionTool(
		"delta_ready_build_v3_all",
		"查询实时卡战备V3市场方案的全部档位汇总数据（数据量较大）。仅当用户要横向对比全部档位时使用。",
		map[string]any{},
		nil,
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			data, err := c.ReadyBuildV3All(ctx)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 15), nil
		},
	))

	// 11. 实时卡战备V3 - 军需处兑换+市场
	reg.Register(ai.NewFunctionTool(
		"delta_ready_build_v3_plus",
		"查询实时卡战备V3方案（军需处兑换+交易行混合），按档位返回配装，通常比纯市场更省。",
		map[string]any{
			"lv": ai.EnumIntProp("战备档位：0=11W 1=18W 2=55W巴克什 3=60W航天 5=78W监狱", 0, 1, 2, 3, 5),
		},
		nil,
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := decodeArgs[ReadyBuildParams](raw)
			if err != nil {
				return nil, err
			}
			data, err := c.ReadyBuildV3Plus(ctx, p)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 20), nil
		},
	))

	// 12. 实时卡战备V3 - 兑换+市场 - ALL
	reg.Register(ai.NewFunctionTool(
		"delta_ready_build_v3_plus_all",
		"查询实时卡战备V3兑换+市场方案的全部档位汇总数据（数据量较大）。仅当用户要横向对比全部档位时使用。",
		map[string]any{},
		nil,
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			data, err := c.ReadyBuildV3PlusAll(ctx)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 15), nil
		},
	))

	// ---------------- delta_api3.md：制造 / 兑换 / 分析 / 改枪码 / 任务 / 密码 / 市集 ----------------

	// 13. 特勤处制造利润Pro
	reg.Register(ai.NewFunctionTool(
		"delta_manufacture_profit",
		"查询特勤处制造利润Pro（自有源成本计算），10分钟更新。用户问“造什么赚钱、工作台/制药台/防具台利润”时使用。",
		map[string]any{
			"t": ai.EnumIntProp("制造台类型：1技术中心 2工作台 3制药台 4防具台", 1, 2, 3, 4),
			"l": ai.IntProp("部门等级"),
		},
		nil,
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := decodeArgs[ManufactureParams](raw)
			if err != nil {
				return nil, err
			}
			data, err := c.ManufacturePro(ctx, p)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 30), nil
		},
	))

	// 14. 军需处兑换 - 成本
	reg.Register(ai.NewFunctionTool(
		"delta_exchange_cost",
		"查询军需处兑换的兑换成本（按部门），10分钟更新。",
		map[string]any{
			"t": ai.EnumIntProp("部门类型：1战斗部门 2医疗部门 3后勤部门 4战术部门 5研发部门", 1, 2, 3, 4, 5),
		},
		nil,
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			var p struct {
				Type int `json:"t"`
			}
			if err := decodeArgsInto(raw, &p); err != nil {
				return nil, err
			}
			data, err := c.Exchange(ctx, p.Type)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 30), nil
		},
	))

	// 15. 军需处兑换 - 子弹收益
	reg.Register(ai.NewFunctionTool(
		"delta_exchange_ammo_profit",
		"查询军需处兑换兑换子弹再出售的收益排行，10分钟更新。用户问“兑换什么子弹赚钱”时使用。",
		map[string]any{},
		nil,
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			data, err := c.ExchangePay(ctx)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 30), nil
		},
	))

	// 16. 军需处兑换 - 战备假账
	reg.Register(ai.NewFunctionTool(
		"delta_exchange_ready_fake",
		"查询军需处兑换物品的战备假账数据（jiazhang假账值，战备值=price+jiazhang），10分钟更新。",
		map[string]any{},
		nil,
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			data, err := c.ExchangeZb(ctx)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 30), nil
		},
	))

	// 17. 今日钥匙卡低价预测
	reg.Register(ai.NewFunctionTool(
		"delta_keys_day_price",
		"查询今日钥匙卡低价买入预测（每天0点更新）。用户问“今天买什么钥匙卡、钥匙卡价格走势”时使用。",
		map[string]any{
			"mid": ai.EnumIntProp("地图（必填）：1大坝 2航天 3长弓 4巴克什 5潮汐监狱", 1, 2, 3, 4, 5),
		},
		[]string{"mid"},
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			var p struct {
				Mid int `json:"mid"`
			}
			if err := decodeArgsInto(raw, &p); err != nil {
				return nil, err
			}
			data, err := c.KeysDay(ctx, p.Mid)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 30), nil
		},
	))

	// 18. 子弹自选包收益
	reg.Register(ai.NewFunctionTool(
		"delta_ammo_pack_profit",
		"按品质等级查询子弹自选包的收益数据。",
		map[string]any{
			"grade": ai.EnumIntProp("子弹品质等级（必填）0~6", 0, 1, 2, 3, 4, 5, 6),
		},
		[]string{"grade"},
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			var p struct {
				Grade int `json:"grade"`
			}
			if err := decodeArgsInto(raw, &p); err != nil {
				return nil, err
			}
			data, err := c.AmmoPack(ctx, p.Grade)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 30), nil
		},
	))

	// 19. 今日倒子弹低价预测
	reg.Register(ai.NewFunctionTool(
		"delta_ammo_day_price",
		"按品质等级查询今日倒子弹（低买高卖）低价预测数据。",
		map[string]any{
			"grade": ai.EnumIntProp("子弹品质等级（必填）0~6", 0, 1, 2, 3, 4, 5, 6),
		},
		[]string{"grade"},
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			var p struct {
				Grade int `json:"grade"`
			}
			if err := decodeArgsInto(raw, &p); err != nil {
				return nil, err
			}
			data, err := c.AmmoDay(ctx, p.Grade)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 30), nil
		},
	))

	// 20. 改枪码 - 烽火
	reg.Register(ai.NewFunctionTool(
		"delta_gun_build_code",
		"查询改枪码（烽火方案），支持按枪械分类/枪械名称搜索和翻页。用户问“某把枪的改枪码/配件方案”时使用。",
		map[string]any{
			"p":     ai.IntProp("页码，默认1"),
			"mtype": ai.StrProp("枪械分类，如 步枪/冲锋枪/狙击步枪"),
			"gun":   ai.StrProp("枪械名称，如 M4A1"),
		},
		nil,
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := decodeArgs[GunBuildCodeParams](raw)
			if err != nil {
				return nil, err
			}
			data, err := c.GunBuildCode(ctx, p)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 20), nil
		},
	))

	// 21. 赛季任务 S11
	reg.Register(ai.NewFunctionTool(
		"delta_season_task_s11",
		"【单次消耗10点资源】查询当前赛季（S11）全部赛季任务数据。用户问赛季任务进度/攻略清单时使用。",
		map[string]any{},
		nil,
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			data, err := c.SeasonTaskS11(ctx)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 30), nil
		},
	))

	// 22. 今日密码
	reg.Register(ai.NewFunctionTool(
		"delta_map_password",
		"查询今日地图密码（每日更新的开箱密码）。用户问“今日密码/今天密码是多少”时使用。",
		map[string]any{},
		nil,
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			return c.MapPassword(ctx)
		},
	))

	// 23. 当前市集所需物品
	reg.Register(ai.NewFunctionTool(
		"delta_market_items",
		"查询当前市集（跳蚤市场/市集任务）所需物品清单。用户问“市集要什么、市集收什么”时使用。",
		map[string]any{},
		nil,
		func(ctx context.Context, raw json.RawMessage) (any, error) {
			data, err := c.Market(ctx)
			if err != nil {
				return nil, err
			}
			return compactResult(data, 30), nil
		},
	))
}

// ---- 工具层通用辅助 ----

// decodeArgs 把模型生成的 JSON 入参解码到强类型结构，空入参返回零值
func decodeArgs[T any](raw json.RawMessage) (T, error) {
	var p T
	if err := decodeArgsInto(raw, &p); err != nil {
		return p, err
	}
	return p, nil
}

// decodeArgsInto 解码到指定指针，空入参视为空对象不报错
func decodeArgsInto(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("工具入参JSON格式错误: %w", err)
	}
	return nil
}

// compactResult 解析接口原始 JSON 并对其中过长的数组做截断，控制回传给模型的数据量，
// 截断位置会插入 {"_truncated":...} 提示对象，引导模型改用分页/精确筛选。
func compactResult(raw json.RawMessage, maxArrayItems int) any {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		// 不是合法JSON时原样返回，交给 Agent 的统一大小兜底
		return raw
	}
	return trimValue(value, maxArrayItems)
}

// trimValue 递归遍历对象/数组，超长的对象数组只保留前 maxArrayItems 项
func trimValue(value any, maxArrayItems int) any {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			v[key] = trimValue(child, maxArrayItems)
		}
		return v
	case []any:
		for i := range v {
			v[i] = trimValue(v[i], maxArrayItems)
		}
		if len(v) > maxArrayItems {
			trimmed := make([]any, 0, maxArrayItems+1)
			trimmed = append(trimmed, v[:maxArrayItems]...)
			trimmed = append(trimmed, map[string]any{
				"_truncated": true,
				"_total":     len(v),
				"_returned":  maxArrayItems,
				"_hint":      "结果已截断，请使用分页参数或增加筛选条件后重试",
			})
			return trimmed
		}
		return v
	default:
		return value
	}
}
