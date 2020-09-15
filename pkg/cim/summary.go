package cim

import "fmt"

type Summary struct {
	NewTags     []TagInfo
	DeletedTags []TagInfo
}

func (s Summary) String() string {
	return fmt.Sprintf(`
	#############################################
	### Summary of container image mirror run ###
	#############################################
	### New Tags:
	%#v
	#############################################
	### Deleted Tags:
	%#v
	#############################################
	#############################################
	`, s.NewTags, s.DeletedTags)
}

type TagInfo string

func (t TagInfo) String() string {
	return "#####" + string(t) + "\n"
}
