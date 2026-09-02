package goclikit

// settings is what the [Option] values assemble before [Execute] runs.
type settings struct {
	notFound NotFoundFunc
}

// Option adjusts what [Execute] does beyond running the command tree.
//
// Variadic rather than fields on a struct parameter, so a CLI wanting none of
// it calls Execute with three arguments and a feature added here never reaches
// the ten call sites that did not ask for it.
type Option func(*settings)

// WithNotFound tells [Execute] how to recognize this tool's not-found, which
// is the one thing it cannot work out for itself.
//
// A tool backed by an HTTP API branches on its status code; one backed by a
// local store branches on its own sentinel:
//
//	goclikit.WithNotFound(func(err error) (string, bool) {
//		var apiErr *api.APIError
//		if !errors.As(err, &apiErr) || !apiErr.NotFound() {
//			return "", false
//		}
//		return apiErr.Message, true
//	})
//
// Without it a not-found is an ordinary error and the recovery annotations are
// never read.
func WithNotFound(classify NotFoundFunc) Option {
	return func(s *settings) { s.notFound = classify }
}
