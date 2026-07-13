package service

import (
	"time"

	"github.com/gin-gonic/gin"
)

const openAIFirstTokenStartContextKey = "openai_first_token_start"

func setOpenAIFirstTokenStart(c *gin.Context, startedAt time.Time) {
	if c == nil || startedAt.IsZero() {
		return
	}
	c.Set(openAIFirstTokenStartContextKey, startedAt)
}

func openAIFirstTokenStart(c *gin.Context, fallback time.Time) time.Time {
	if c != nil {
		if value, ok := c.Get(openAIFirstTokenStartContextKey); ok {
			if startedAt, ok := value.(time.Time); ok && !startedAt.IsZero() {
				return startedAt
			}
		}
	}
	return fallback
}
