package defaultpipelines

import (
	"context"
	"sync"

	validator "gopkg.in/go-playground/validator.v9"
)

// DefaultValidator is the default arguments validator for handlers
// in pitaya
type DefaultValidator struct {
	once     sync.Once
	validate *validator.Validate
}

// Validate is the the function responsible for validating the 'in' parameter
// based on the struct tags the parameter has.
// This function has the pipeline.Handler signature so
// it is possible to use it as a pipeline function
func (v *DefaultValidator) Validate(ctx context.Context, in interface{}) (context.Context, interface{}, error, int32) {
	if in == nil {
		return ctx, in, nil, 0
	}

	v.lazyinit()
	if err := v.validate.Struct(in); err != nil {
		return ctx, nil, err, 0
	}

	return ctx, in, nil, 0
}

func (v *DefaultValidator) lazyinit() {
	v.once.Do(func() {
		v.validate = validator.New()
	})
}
