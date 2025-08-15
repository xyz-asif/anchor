package pagination

import (
	"math"
	"strconv"
)

// Pagination represents pagination metadata
type Pagination struct {
	Page    int   `json:"page"`
	Limit   int   `json:"limit"`
	Total   int64 `json:"total"`
	Pages   int   `json:"pages"`
	HasNext bool  `json:"hasNext"`
	HasPrev bool  `json:"hasPrev"`
	Offset  int   `json:"-"`
}

// PaginationRequest represents a pagination request from client
type PaginationRequest struct {
	Page  int `json:"page" form:"page"`
	Limit int `json:"limit" form:"limit"`
}

// New creates a new pagination instance
func New(page, limit int, total int64) *Pagination {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	pages := int(math.Ceil(float64(total) / float64(limit)))
	if pages < 1 {
		pages = 1
	}

	offset := (page - 1) * limit

	return &Pagination{
		Page:    page,
		Limit:   limit,
		Total:   total,
		Pages:   pages,
		HasNext: page < pages,
		HasPrev: page > 1,
		Offset:  offset,
	}
}

// FromRequest creates pagination from HTTP request parameters
func FromRequest(pageStr, limitStr string) *PaginationRequest {
	page, _ := strconv.Atoi(pageStr)
	limit, _ := strconv.Atoi(limitStr)

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	return &PaginationRequest{
		Page:  page,
		Limit: limit,
	}
}

// GetOffset returns the offset for database queries
func (p *Pagination) GetOffset() int {
	return p.Offset
}

// GetLimit returns the limit for database queries
func (p *Pagination) GetLimit() int {
	return p.Limit
}

// IsValid checks if the pagination parameters are valid
func (p *Pagination) IsValid() bool {
	return p.Page > 0 && p.Limit > 0 && p.Page <= p.Pages
}

// GetPageRange returns a slice of page numbers to display (for UI pagination)
func (p *Pagination) GetPageRange(maxPages int) []int {
	if maxPages < 1 {
		maxPages = 5
	}

	var pages []int
	totalPages := p.Pages

	if totalPages <= maxPages {
		// Show all pages
		for i := 1; i <= totalPages; i++ {
			pages = append(pages, i)
		}
	} else {
		// Show a window of pages around current page
		start := p.Page - maxPages/2
		if start < 1 {
			start = 1
		}

		end := start + maxPages - 1
		if end > totalPages {
			end = totalPages
			start = end - maxPages + 1
		}

		for i := start; i <= end; i++ {
			pages = append(pages, i)
		}
	}

	return pages
}

// GetNextPage returns the next page number, or 0 if no next page
func (p *Pagination) GetNextPage() int {
	if p.HasNext {
		return p.Page + 1
	}
	return 0
}

// GetPrevPage returns the previous page number, or 0 if no previous page
func (p *Pagination) GetPrevPage() int {
	if p.HasPrev {
		return p.Page - 1
	}
	return 0
}

// GetFirstPage returns the first page number (always 1)
func (p *Pagination) GetFirstPage() int {
	return 1
}

// GetLastPage returns the last page number
func (p *Pagination) GetLastPage() int {
	return p.Pages
}

// GetStartItem returns the index of the first item on current page (1-based)
func (p *Pagination) GetStartItem() int {
	return p.Offset + 1
}

// GetEndItem returns the index of the last item on current page (1-based)
func (p *Pagination) GetEndItem() int {
	end := p.Offset + p.Limit
	if int64(end) > p.Total {
		end = int(p.Total)
	}
	return end
}

// GetTotalPages returns the total number of pages
func (p *Pagination) GetTotalPages() int {
	return p.Pages
}

// GetTotalItems returns the total number of items
func (p *Pagination) GetTotalItems() int64 {
	return p.Total
}

// GetItemsPerPage returns the number of items per page
func (p *Pagination) GetItemsPerPage() int {
	return p.Limit
}

// GetCurrentPage returns the current page number
func (p *Pagination) GetCurrentPage() int {
	return p.Page
}
