package gfx

import "github.com/kjkrol/gokg/pkg/spatial"

type GridConfig struct {
	WorldResolution        spatial.Resolution
	WorldWrap              bool
	CacheMarginBuckets     int
	DefaultBucketResolution spatial.Resolution
	DefaultBucketCapacity  int
}

func normalizeGridConfig(conf GridConfig, viewWidth, viewHeight int) GridConfig {
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
	if conf.CacheMarginBuckets <= 0 {
		conf.CacheMarginBuckets = 2
	}
	if conf.DefaultBucketResolution == 0 {
		conf.DefaultBucketResolution = spatial.NewResolution(6)
	}
	if conf.DefaultBucketCapacity <= 0 {
		conf.DefaultBucketCapacity = 16
	}
	return conf
}
