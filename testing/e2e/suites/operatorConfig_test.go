package suites

import (
	"reflect"
	"testing"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
)

func Test_cfgBuilderWithOsm_withVersions(t *testing.T) {
	type fields struct {
		cfgBuilder cfgBuilder
		osmEnabled []bool
	}
	type args struct {
		in       infra.Provisioned
		versions []manifests.OperatorVersion
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   cfgBuilderWithVersions
	}{
		// TODO: Add test cases.
		{
			name: "only keep latest for sp",
			fields: fields{
				cfgBuilder: cfgBuilder{},
			},
			args: args{
				in: infra.Provisioned{
					AuthType: infra.AuthTypeServicePrincipal,
				},
				versions: manifests.AllOperatorVersions,
			},
			want: cfgBuilderWithVersions{
				versions: []manifests.OperatorVersion{manifests.OperatorVersionLatest},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := cfgBuilderWithOsm{
				cfgBuilder: tt.fields.cfgBuilder,
				osmEnabled: tt.fields.osmEnabled,
			}
			if got := c.withVersions(tt.args.in, tt.args.versions...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("withVersions() = %v, want %v", got, tt.want)
			}
		})
	}
}
