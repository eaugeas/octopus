package concurrent

// Supplier defines an arbitrary operation
type Supplier interface {
	Supply() (interface{}, error)
}

// SupplierFunc allows a function to act as a Supplier
type SupplierFunc func() (interface{}, error)

// Implementation of Supplier interface for SupplierFunc.
func (f SupplierFunc) Supply() (interface{}, error) {
	return f()
}

// Result of a supplier
type Result struct {
	value interface{}
	err   error
}

// Value returned by the supplier
func (r Result) Value() interface{} {
	return r.value
}

// Err returned by the supplier
func (r Result) Err() error {
	return r.err
}
