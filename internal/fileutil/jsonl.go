package fileutil

import (
	"bytes"
	"encoding/json"
)

func EncodeJSONL[T any](records []T) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}
