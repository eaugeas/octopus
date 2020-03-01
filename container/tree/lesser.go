package tree

// Lesser compares two values
type Lesser interface {
	// Less returns
	//  -1 if a < b
	//   0 if a == b
	//   1 if a > b
	Less(a, b interface{}) int
}

// IntLesser implementation of the Lesser interface for
// integers
type IntLesser struct{}

// Less returns
//  -1 if a < b
//   0 if a == b
//   1 if a > b
func (IntLesser) Less(a, b interface{}) int {
	if a.(int) < b.(int) {
		return -1
	} else if a.(int) > b.(int) {
		return 1
	} else {
		return 0
	}
}
