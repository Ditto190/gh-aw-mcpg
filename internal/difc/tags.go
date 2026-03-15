package difc

import "strings"

// StringsToTags converts a slice of strings to a slice of Tags,
// trimming whitespace and skipping empty values.
func StringsToTags(values []string) []Tag {
	tags := make([]Tag, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			tags = append(tags, Tag(trimmed))
		}
	}
	return tags
}

// TagsToStrings converts a slice of Tags to a slice of strings.
func TagsToStrings(tags []Tag) []string {
	values := make([]string, 0, len(tags))
	for _, tag := range tags {
		values = append(values, string(tag))
	}
	return values
}
