package gfx

import "github.com/kjkrol/gokg/pkg/spatial"

type WorldConfig struct {
	WorldResolution spatial.Resolution
	WorldWrap       bool
}

func normalizeWorldConfig(conf WorldConfig, viewWidth, viewHeight int) WorldConfig {
	if conf.WorldResolution == 0 {
		maxSide := viewWidth
		if viewHeight > maxSide {
			maxSide = viewHeight
		}
		if maxSide <= 0 {
			conf.WorldResolution = spatial.Size1x1
		} else {
			conf.WorldResolution = spatial.ResolutionFrom(uint32(maxSide - 1))
		}
	}
	return conf
}
