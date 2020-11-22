package config

import (
	"os"
	"strconv"
	"strings"
)

func GetEnv(key string, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}

	return defaultValue
}

func GetEnvOrPanic(key string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}

	panic("Can't find environment variable " + key)
}

func GetEnvAsInt(name string, defaultValue int) int {
	valueStr := GetEnv(name, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}

	return defaultValue
}

func GetEnvAsFloat64(name string, defaultValue float64) float64 {
	valueStr := GetEnv(name, "")
	if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
		return value
	}

	return defaultValue
}

func GetEnvAsBool(name string, defaultValue bool) bool {
	valStr := GetEnv(name, "")
	if val, err := strconv.ParseBool(valStr); err == nil {
		return val
	}

	return defaultValue
}

func GetEnvAsSlice(name string, defaultValue []string, separator string) []string {
	strValue := GetEnv(name, "")
	if strValue == "" {
		return defaultValue
	}
	value := strings.Split(strValue, separator)
	return value
}
