package main

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var _ provider.Provider = &NamingProvider{}

// NamingProvider is a schema-driven Azure/CAF resource naming provider. It
// has no configuration: the `naming_schema` resource resolves a naming
// schema and persists a unique id.
type NamingProvider struct {
	version string
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &NamingProvider{version: version}
	}
}

func (p *NamingProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "naming"
	resp.Version = p.version
}

func (p *NamingProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Schema-driven Azure/CAF resource naming. The `naming_schema` resource resolves a naming schema and persists a unique id. This provider takes no configuration.",
	}
}

func (p *NamingProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
}

func (p *NamingProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewSchemaResource,
	}
}

func (p *NamingProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return nil
}
