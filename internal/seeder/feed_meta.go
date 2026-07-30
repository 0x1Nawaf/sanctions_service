package seeder

import (
	"encoding/json"
	"os"
)

type feedMeta struct {
	FeedScope   string `json:"feed_scope"`
	RecordCount int    `json:"record_count"`
}

func readFeedMeta(jsonPath string) (feedMeta, error) {
	f, err := os.Open(jsonPath)
	if err != nil {
		return feedMeta{}, err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	tok, err := dec.Token()
	if err != nil {
		return feedMeta{}, err
	}
	if tok != json.Delim('{') {
		return feedMeta{}, nil
	}

	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return feedMeta{}, err
		}
		key, _ := tok.(string)
		if key == "_meta" {
			var meta feedMeta
			if err := dec.Decode(&meta); err != nil {
				return feedMeta{}, err
			}
			return meta, nil
		}
		if err := skipJSONDecoderValue(dec); err != nil {
			return feedMeta{}, err
		}
	}
	return feedMeta{}, nil
}

func skipJSONDecoderValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}

	switch delim {
	case '[':
		for dec.More() {
			if err := skipJSONDecoderValue(dec); err != nil {
				return err
			}
		}
		_, err = dec.Token()
	case '{':
		for dec.More() {
			if _, err = dec.Token(); err != nil {
				return err
			}
			if err := skipJSONDecoderValue(dec); err != nil {
				return err
			}
		}
		_, err = dec.Token()
	}
	return err
}

func (s *Seeder) isCompleteFeed(incoming, existing int) bool {
	if s.feedMeta.FeedScope == "delta_only" {
		return false
	}
	if s.feedMeta.FeedScope == "complete" {
		return true
	}
	if existing > 10_000 && incoming < existing*9/10 {
		return false
	}
	if s.feedMeta.RecordCount > 0 && existing > 10_000 && s.feedMeta.RecordCount < existing*9/10 {
		return false
	}
	return true
}

func recordNeedsUpdate(ex existingRecord, row map[string]interface{}) bool {
	newActive := strString(row, "active_status")
	if ex.activeStatus != newActive {
		return true
	}
	if strString(row, "action") == "del" && isActiveStatus(ex.activeStatus) {
		return true
	}
	return false
}
