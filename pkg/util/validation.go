package util

import "regexp"

// IsValidEmail ...
func IsValidEmail(email string) bool {
	r := regexp.MustCompile(`^[\w-\.]+@([\w-]+\.)+[\w-]{2,4}$`)
	return r.MatchString(email)
}

// IsValidPhone ...
func IsValidPhone(phone string) bool {
	r := regexp.MustCompile(`^998[0-9]{2}[0-9]{7}$`)
	return r.MatchString(phone)
}
