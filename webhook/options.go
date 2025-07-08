package webhook

import "time"

var defaultOptions = Options{
	address:        ":9567",
	supportVersion: "4",
	gracePeriod:    15 * time.Second,
}

type Options struct {
	address        string
	supportVersion string
	gracePeriod    time.Duration
}

type Option interface {
	apply(*Options)
}

type optionFunc func(*Options)

func (f optionFunc) apply(o *Options) {
	f(o)
}

func WithAddress(address string) optionFunc {
	return optionFunc(func(o *Options) {
		o.address = address
	})
}

func WithGracePeriod(gracePeriod time.Duration) optionFunc {
	return optionFunc(func(o *Options) {
		o.gracePeriod = gracePeriod
	})
}

func WithSupportVersion(version string) optionFunc {
	return optionFunc(func(o *Options) {
		o.supportVersion = version
	})
}
