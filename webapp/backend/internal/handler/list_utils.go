package handler

import (
	"strings"

	"backend/internal/model"
)

// sanitizeListRequest applies allowlists for sort field/order and defaults.
func sanitizeListRequest(req *model.ListRequest, allowedFields map[string]string, defaultField, defaultOrder string) {
	fieldKey := strings.ToLower(req.SortField)
	if fieldKey == "" {
		req.SortField = defaultField
	} else if mapped, ok := allowedFields[fieldKey]; ok {
		req.SortField = mapped
	} else {
		req.SortField = defaultField
	}

	order := strings.ToLower(req.SortOrder)
	switch order {
	case "asc", "desc":
		req.SortOrder = order
	default:
		req.SortOrder = strings.ToLower(defaultOrder)
	}
}
