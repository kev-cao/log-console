package sliceutils

// Map applies a function to each element of a slice and returns a new slice with the results.
func Map[T, U any](s []T, f func(T, int) U) []U {
	result := make([]U, len(s))
	for i, v := range s {
		result[i] = f(v, i)
	}
	return result
}

// MapErr applies a function to each element of a slice and returns a new slice with the results.
// If the function returns an error, the function stops and returns the error.
func MapErr[T, U any](s []T, f func(T, int) (U, error)) ([]U, error) {
	result := make([]U, len(s))
	for i, v := range s {
		var err error
		result[i], err = f(v, i)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
