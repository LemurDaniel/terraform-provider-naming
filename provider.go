package main

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/LemurDaniel/terraform-provider-naming/internal/schemaengine"
)

var _ provider.Provider = &NamingProvider{}

// NamingProvider is a schema-driven Azure/CAF resource naming provider. The
// naming convention is configured once on the provider, and the
// `naming_name` data source renders names from it.
type NamingProvider struct {
	version string
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &NamingProvider{version: version}
	}
}

// namingProviderModel is the Terraform representation of the provider's own
// configuration block.
type namingProviderModel struct {
	Convention types.String `tfsdk:"convention"`
	Naming     types.String `tfsdk:"naming"`
	Parameters types.Map    `tfsdk:"parameters"`
	RandomSeed types.String `tfsdk:"random_seed"`
}

func (p *NamingProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "naming"
	resp.Version = p.version
}

func (p *NamingProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Schema-driven Azure/CAF resource naming. The naming convention is declared once here - a bundled convention, an optional YAML override, and the runtime parameters - and the [`naming_name`](data-sources/name.md) data source renders names from it.",
		Attributes: map[string]schema.Attribute{
			"convention": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Name of the bundled convention to load (e.g. `\"default\"`). Defaults to `\"default\"`.",
			},
			"naming": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Path to a YAML file that partially overrides the bundled convention (e.g. `\"${path.module}/naming.yaml\"`). Every table is merged per key with your value winning, so an override may set a single abbreviation or pattern without restating the block around it.",
			},
			"parameters": schema.MapAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Runtime parameters (`location`, `environment`, `name`, ...) that patterns render from. Layered on top of the convention's `default_parameters`, and overridable per call on the data source.",
			},
			"random_seed": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Seed for the `<UNIQUE_ID_n>` pattern token. The same seed always resolves to the same id, so names stay stable across plans and applies without any state. Required only if a pattern you use actually contains the token.",
			},
		},
	}
}

// Configure resolves the naming schema once and hands it to every data
// source. Resolving is pure and local - embedded YAML plus in-memory merging
// - so it needs no client, no network and no state.
func (p *NamingProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config namingProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The schema has to exist before any data source is read, so a value
	// that is only known after apply cannot be used here.
	for attribute, unknown := range map[string]bool{
		"convention":  config.Convention.IsUnknown(),
		"naming":      config.Naming.IsUnknown(),
		"parameters":  config.Parameters.IsUnknown(),
		"random_seed": config.RandomSeed.IsUnknown(),
	} {
		if unknown {
			resp.Diagnostics.AddAttributeError(
				path.Root(attribute),
				"Unknown provider configuration",
				"The naming provider resolves its schema while configuring, so "+attribute+" must be known at plan time. Avoid deriving it from a resource attribute that is only known after apply.",
			)
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	parameters := make(map[string]interface{}, len(config.Parameters.Elements()))
	if !config.Parameters.IsNull() {
		var raw map[string]string
		resp.Diagnostics.Append(config.Parameters.ElementsAs(ctx, &raw, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		for key, value := range raw {
			parameters[key] = value
		}
	}

	resolved, err := schemaengine.Resolve(schemaengine.Config{
		Convention: config.Convention.ValueString(),
		NamingFile: config.Naming.ValueString(),
		Parameters: parameters,
		RandomSeed: config.RandomSeed.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to resolve the naming schema", err.Error())
		return
	}

	resp.DataSourceData = resolved
	resp.ResourceData = resolved
}

func (p *NamingProvider) Resources(ctx context.Context) []func() resource.Resource {
	return nil
}

func (p *NamingProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewNameDataSource,
		NewSchemaDataSource,
	}
}
