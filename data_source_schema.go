package main

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/LemurDaniel/terraform-provider-naming/internal/schemaengine"
)

var _ datasource.DataSource = &SchemaDataSource{}
var _ datasource.DataSourceWithConfigure = &SchemaDataSource{}

// SchemaDataSource exposes the provider's resolved naming schema - the
// merged abbreviations, patterns, mappings and settings - for inspection and
// for consumers that want to render a name themselves.
type SchemaDataSource struct {
	schema *schemaengine.Schema
}

func NewSchemaDataSource() datasource.DataSource {
	return &SchemaDataSource{}
}

// schemaDataSourceModel is the Terraform representation of naming_schema.
// Every table is dynamic because the YAML mixes strings and nested objects
// at the same level by design.
type schemaDataSourceModel struct {
	IndexModifier     types.Dynamic `tfsdk:"index_modifier"`
	EnforceLowerCase  types.Dynamic `tfsdk:"enforce_lower_case"`
	Abbreviations     types.Dynamic `tfsdk:"abbreviations"`
	Mappings          types.Dynamic `tfsdk:"mappings"`
	Patterns          types.Dynamic `tfsdk:"patterns"`
	DefaultParameters types.Dynamic `tfsdk:"default_parameters"`
	UniqueID          types.String  `tfsdk:"unique_id"`
}

func (d *SchemaDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schema"
}

func (d *SchemaDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The naming schema resolved from the provider's configuration: the bundled convention with any override merged over it. Useful for inspecting what a convention resolves to, or for rendering a name in HCL instead of through [`naming_name`](name.md).",
		Attributes: map[string]schema.Attribute{
			"index_modifier": schema.DynamicAttribute{
				Computed:            true,
				MarkdownDescription: "The resolved index offset (e.g. `1`, so rendered indices start at `01`).",
			},
			"enforce_lower_case": schema.DynamicAttribute{
				Computed:            true,
				MarkdownDescription: "The resolved lower-casing settings.",
			},
			"abbreviations": schema.DynamicAttribute{
				Computed:            true,
				MarkdownDescription: "The merged abbreviations table, keyed `<provider>` then `\"<Namespace>/<Type>::<kind>\"`.",
			},
			"mappings": schema.DynamicAttribute{
				Computed:            true,
				MarkdownDescription: "The merged mappings table (e.g. location and environment shortnames).",
			},
			"patterns": schema.DynamicAttribute{
				Computed:            true,
				MarkdownDescription: "The merged patterns table.",
			},
			"default_parameters": schema.DynamicAttribute{
				Computed:            true,
				MarkdownDescription: "The convention's default parameters with the provider's `parameters` layered on top.",
			},
			"unique_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID that `<UNIQUE_ID_n>` renders from, derived from the provider's `random_seed`. Empty when no seed is configured.",
			},
		},
	}
}

func (d *SchemaDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	resolved, ok := req.ProviderData.(*schemaengine.Schema)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data",
			"The naming provider did not supply a resolved schema. This is a bug in the provider.",
		)
		return
	}

	d.schema = resolved
}

func (d *SchemaDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.schema == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The naming provider has no resolved schema. Add a `provider \"naming\" {}` block to the configuration.",
		)
		return
	}

	var state schemaDataSourceModel

	set := func(name string, value interface{}, target *types.Dynamic) {
		dynamic, err := schemaengine.ToDynamic(value)
		if err != nil {
			resp.Diagnostics.AddError("Failed to encode "+name, err.Error())
			return
		}
		*target = dynamic
	}

	set("index_modifier", d.schema.IndexModifier, &state.IndexModifier)
	set("enforce_lower_case", d.schema.EnforceLowerCase, &state.EnforceLowerCase)
	set("abbreviations", d.schema.Abbreviations, &state.Abbreviations)
	set("mappings", d.schema.Mappings, &state.Mappings)
	set("patterns", d.schema.Patterns, &state.Patterns)
	set("default_parameters", d.schema.DefaultParameters, &state.DefaultParameters)
	if resp.Diagnostics.HasError() {
		return
	}

	state.UniqueID = types.StringValue(d.schema.UniqueID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
