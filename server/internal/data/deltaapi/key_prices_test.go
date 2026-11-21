package deltaapi

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDailyKeyPricesKeepsPerCardMinimum(t *testing.T) {
	input := json.RawMessage(`[
		{"hour":11,"data":[{"name":"金卡","price":3930000},{"name":"西区大门","price":880000}]},
		{"hour":5,"data":[{"name":"金卡","price":4680000},{"name":"西区大门","price":800000}]},
		{"hour":0,"data":[{"name":"金卡","price":3930000},{"name":"缺价","price":0}]}
	]`)
	got, err := dailyKeyPrices(input, 2)
	want := []DailyKeyPrice{{Name: "西区大门", Price: 800000, Hour: 5}, {Name: "金卡", Price: 3930000, Hour: 0}}
	if err != nil || got.Map != "航天基地" || !reflect.DeepEqual(got.Cards, want) {
		t.Fatalf("got %+v, err %v", got, err)
	}
}
