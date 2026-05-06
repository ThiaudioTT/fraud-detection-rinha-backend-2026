package handlers

import "github.com/gin-gonic/gin"

func Ready(c *gin.Context) {

	c.JSON(200, gin.H{
		"message": "ready",
	})
}
