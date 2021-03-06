package webhttp

import (
	"testing"

	"github.com/saarasio/enroute/enroute-dp/saarasconfig"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestSetRouteConfigJson(t *testing.T) {
	tests := map[string]struct {
		route_config string
		want         saarasconfig.RouteMatchConditions
	}{

		"GET with header prefix /": {
			route_config: `
            {
                "Prefix" : "/",  
                "header":
                [
                { "header_name" : ":method", "header_value" : "GET"}
                ]
            }
            `,
			want: saarasconfig.RouteMatchConditions{
				Prefix: "/",
				MatchConditions: []saarasconfig.RouteMatchCondition{{
					HeaderName:  ":method",
					HeaderValue: "GET",
				}},
			},
		},
		"GET with two headers prefix /": {
			route_config: `
            {
                "Prefix" : "/",  
                "header":
                [
                { "header_name" : ":method", "header_value" : "GET"},
                { "header_name" : ":path", "header_value" : "/"}
                ]
            }
            `,
			want: saarasconfig.RouteMatchConditions{
				Prefix: "/",
				MatchConditions: []saarasconfig.RouteMatchCondition{{
					HeaderName:  ":method",
					HeaderValue: "GET",
				},
					{
						HeaderName:  ":path",
						HeaderValue: "/",
					},
				},
			},
		},

		"Empty config": {
			route_config: `
        {
            "header": []
        }
        `,
			want: saarasconfig.RouteMatchConditions{
				MatchConditions: []saarasconfig.RouteMatchCondition{},
			},
		},
		"No config": {
			route_config: `{}`,
			want: saarasconfig.RouteMatchConditions{
				MatchConditions: nil,
			},
		},
		"prefix /": {
			route_config: `
        {
            "Prefix" : "/",  
            "header":
            [ {} ]
        }
        `,
			want: saarasconfig.RouteMatchConditions{
				Prefix:          "/",
				MatchConditions: []saarasconfig.RouteMatchCondition{{}},
			},
		},
		"prefix /blah": {
			route_config: `
        {
            "Prefix" : "/blah",
            "header":
            [ {} ]
        }
        `,
			want: saarasconfig.RouteMatchConditions{
				Prefix:          "/blah",
				MatchConditions: []saarasconfig.RouteMatchCondition{{}},
			},
		},
	}

	args := make(map[string]interface{})
	log2 := logrus.StandardLogger()
	log := log2.WithField("context", "web-http")
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			setRouteConfigJson(log, tc.route_config, &args)
			got := args["config_json"]
			assert.Equal(t, tc.want, got)
		})
	}
}
