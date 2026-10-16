package main

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/LemurDaniel/terraform-provider-naming/internal/schemaengine"
)

var _ datasource.DataSource = &NameDataSource{}
var _ datasource.DataSourceWithConfigure = &NameDataSource{}

// NameDataSource renders one or more resource names from the provider's
// resolved naming schema. It is a data source rather than a resource because
// rendering is pure: the same schema and the same arguments always produce
// the same names, and nothing needs persisting.
type NameDataSource struct {
	schema *schemaengine.Schema
}

func NewNameDataSource() datasource.DataSource {
	return &NameDataSource{}
}

// nameDataSourceModel is the Terraform representation of naming_name.
type nameDataSourceModel struct {
	Resource   types.String `tfsdk:"resource"`
	NamingID   types.String `tfsdk:"naming_id"`
	Index      types.Object `tfsdk:"index"`
	Parameters types.Map    `tfsdk:"parameters"`

	Name    types.String `tfsdk:"name"`
	Result  types.String `tfsdk:"result"`
	AtIndex types.List   `tfsdk:"at_index"`
	ByIndex types.List   `tfsdk:"by_index"`
}

// nameIndexModel is the `index` argument: where to start numbering and how
// many names to render.
type nameIndexModel struct {
	Start types.Int64 `tfsdk:"start"`
	Count types.Int64 `tfsdk:"count"`
}

func (d *NameDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_name"
}

func (d *NameDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Renders one or more resource names from the naming schema configured on the provider.",
		Attributes: map[string]schema.Attribute{
			"resource": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The resource to name, as `\"<Provider>::<Namespace>/<Type>[::<kind>]\"` - e.g. `\"Azure::Microsoft.Storage/storageAccounts\"` or `\"Azure::Microsoft.Compute/disks::os\"`. The optional `kind` selects a variant of the same type and defaults to `default`.",
			},
			"naming_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Selects a named variant of the pattern and abbreviation. Takes precedence over the resource's `kind`.",
			},
			"index": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "The index range to render. Omitted, a single name is rendered at index `start = 0`.",
				Attributes: map[string]schema.Attribute{
					"start": schema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "The first index. Defaults to `0`, which the convention's `index_modifier` shifts (so the first name is numbered `01` by default).",
					},
					"count": schema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "How many names to render. Defaults to `1`.",
					},
				},
			},
			"parameters": schema.MapAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Parameters for this call, layered on top of the provider's `parameters` and the convention's `default_parameters`.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The first rendered name.",
			},
			"result": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Alias for `name`.",
			},
			"at_index": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Every rendered name, in index order - `index.count` of them.",
			},
			"by_index": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Alias for `at_index`.",
			},
		},
	}
}

func (d *NameDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *NameDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config nameDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.schema == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The naming provider has no resolved schema. Add a `provider \"naming\" {}` block to the configuration.",
		)
		return
	}

	resourceID, err := schemaengine.ParseResourceID(config.Resource.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("resource"), "Invalid resource identifier", err.Error())
		return
	}

	// index is optional, so an absent block means one name starting at 0.
	start, count := int64(0), int64(1)
	if !config.Index.IsNull() && !config.Index.IsUnknown() {
		var index nameIndexModel
		resp.Diagnostics.Append(config.Index.As(ctx, &index, basetypes.ObjectAsOptions{})...)
		if resp.Diagnostics.HasError() {
			return
		}
		if !index.Start.IsNull() {
			start = index.Start.ValueInt64()
		}
		if !index.Count.IsNull() {
			count = index.Count.ValueInt64()
		}
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

	names, err := d.schema.Generate(schemaengine.Request{
		Resource:   resourceID,
		NamingID:   config.NamingID.ValueString(),
		IndexStart: int(start),
		IndexCount: int(count),
		Parameters: parameters,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to render the name", err.Error())
		return
	}

	nameList, diags := types.ListValueFrom(ctx, types.StringType, names)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config.Name = types.StringValue(names[0])
	config.Result = config.Name
	config.AtIndex = nameList
	config.ByIndex = nameList

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
