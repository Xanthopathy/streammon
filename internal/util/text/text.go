package text

import (
	"fmt"
	"regexp"
	"strings"
)

// SanitizeFilename replaces invalid characters with underscores (matches python logic)
func SanitizeFilename(name string) string {
	re := regexp.MustCompile(`[<>:"/\\|?*]`)
	return re.ReplaceAllString(name, "_")
}

// SanitizeFolderName converts to lowercase, replaces spaces with _, and removes invalid chars
func SanitizeFolderName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "_")
	return SanitizeFilename(name)
}

// JoinCommandArgs joins command arguments into a single string, quoting args that contain spaces
func JoinCommandArgs(args []string) string {
	var result []string
	for _, arg := range args {
		if strings.Contains(arg, " ") {
			result = append(result, fmt.Sprintf(`"%s"`, arg))
		} else {
			result = append(result, arg)
		}
	}
	return strings.Join(result, " ")
}
