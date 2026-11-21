package deltaapi

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type DailyKeyPrice struct {
	Name  string `json:"name"`
	Price int64  `json:"low_price"`
	Hour  int    `json:"hour"`
}

type DailyKeyPrices struct {
	Map   string          `json:"map"`
	Kind  string          `json:"kind"`
	Cards []DailyKeyPrice `json:"cards"`
}

// dailyKeyPrices projects the hourly predictions into one comparable row per card.
func dailyKeyPrices(raw json.RawMessage, mapID int) (DailyKeyPrices, error) {
	maps := [...]string{"", "零号大坝", "航天基地", "长弓溪谷", "巴克什", "潮汐监狱"}
	if mapID < 1 || mapID >= len(maps) {
		return DailyKeyPrices{}, fmt.Errorf("未识别的地图编号 %d", mapID)
	}
	var periods []struct {
		Hour  *int `json:"hour"`
		Cards []struct {
			Name  string `json:"name"`
			Price int64  `json:"price"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &periods); err != nil {
		return DailyKeyPrices{}, err
	}
	best := map[string]DailyKeyPrice{}
	for _, period := range periods {
		if period.Hour == nil || *period.Hour < 0 || *period.Hour > 23 {
			continue
		}
		for _, card := range period.Cards {
			name := strings.TrimSpace(card.Name)
			if name == "" || card.Price <= 0 {
				continue
			}
			previous, found := best[name]
			if !found || card.Price < previous.Price || (card.Price == previous.Price && *period.Hour < previous.Hour) {
				best[name] = DailyKeyPrice{Name: name, Price: card.Price, Hour: *period.Hour}
			}
		}
	}
	result := DailyKeyPrices{Map: maps[mapID], Kind: "预测低价，非实时成交价", Cards: make([]DailyKeyPrice, 0, len(best))}
	for _, card := range best {
		result.Cards = append(result.Cards, card)
	}
	sort.Slice(result.Cards, func(i, j int) bool {
		left, right := result.Cards[i], result.Cards[j]
		if left.Price != right.Price {
			return left.Price < right.Price
		}
		return left.Name < right.Name
	})
	return result, nil
}
