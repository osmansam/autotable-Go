package main

import (
	"github.com/gofiber/fiber/v2"
	"strconv"
)

func Subtract(c *fiber.Ctx) (interface{}, error) {
	aStr := c.Query("a")
	bStr := c.Query("b")
	a, err := strconv.Atoi(aStr)
	if err != nil {
		return nil, err
	}
	b, err := strconv.Atoi(bStr)
	if err != nil {
		return nil, err
	}
	sum := a - b
	return sum, nil
}