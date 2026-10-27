package core

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"testing"
)

func TestHongguoFeedRecommendationsKeepSessionAndSkipRepeats(t *testing.T) {
	pages := map[string][]string{"0": {"9001", "9002"}, "1": {"9002"}, "2": {"9003"}}
	for _, feed := range hongguoRecommendationFeeds {
		t.Run(feed.key, func(t *testing.T) {
			var offsets []string
			transport := sourceFixtureTransport(func(request *http.Request) (*http.Response, error) {
				query := request.URL.Query()
				if request.URL.Path != "/reading/bookapi/bookmall/tab/v1/" || query.Get("tab_type") != feed.tabType || query.Get("bottom_tab_type") != feed.bottomTab {
					t.Errorf("unexpected request: %s %v", request.URL.Path, query)
				}
				offset := query.Get("offset")
				if offset != "0" && (query.Get("session_id") != "fixture-feed" || query.Get("client_req_type") != "2") {
					t.Error("continuation lost the feed session", query)
				}
				offsets = append(offsets, offset)
				cards := []any{
					map[string]any{"show_type": 571, "user_research": map[string]any{}},
					map[string]any{"group_id_type": 27, "video_data": []any{map[string]any{}}},
				}
				for _, id := range pages[offset] {
					row := map[string]any{"series_id": id, "title": "合成推荐 " + id}
					if feed.bottomTab != "" {
						row = map[string]any{"series_id": id, "video_detail": map[string]any{"series_id": id, "series_title": "合成推荐 " + id}}
					}
					cards = append(cards, map[string]any{"group_id_type": 9, "video_data": []any{row}})
				}
				tabType, _ := strconv.Atoi(feed.tabType)
				body, _ := json.Marshal(map[string]any{"code": 0, "data": map[string]any{"tab_item": []any{
					map[string]any{"tab_type": 45, "title": "其他"},
					map[string]any{"tab_type": tabType, "session_id": "fixture-feed", "cell_data": []any{map[string]any{"cell_data": cards}}},
				}}})
				return sourceFixtureResponse(request, http.StatusOK, string(body)), nil
			})
			engine := sourceFixtureEngine(t, transport)
			first, err := engine.nativeRecommendations(context.Background(), nativeInput{Category: feed.key})
			if err != nil || len(first.Items) != 2 || !first.HasMore || first.Items[0].Title != "合成推荐 9001" {
				t.Fatal("first feed page was not parsed", err, first.Items)
			}
			more, err := engine.nativeRecommendations(context.Background(), nativeInput{Category: feed.key, Command: "more"})
			if err != nil || len(more.Items) != 3 || more.Items[2].Title != "合成推荐 9003" {
				t.Fatal("continuation did not skip the repeated page", err, more.Items)
			}
			if !reflect.DeepEqual(offsets, []string{"0", "1", "2"}) {
				t.Fatal("feed offsets did not advance by page", offsets)
			}
		})
	}
}
