package main

import (
	"context"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/LemurDaniel/terraform-provider-naming/internal/schemaengine"
)

// namingSeedNamespace is an arbitrary, fixed namespace used to derive
// deterministic (v5) UUIDs from a user-supplied random_seed. It has no
// meaning beyond making seeded UUIDs unique to this provider.
var namingSeedNamespace = uuid.MustParse("d29c7fb1-4c07-4f3c-9d90-14a765d5f2f0")

func deterministicUUID(seed string) string {
	return uuid.NewSHA1(namingSeedNamespace, []byte(seed)).String()
}

var _ resource.Resource = &SchemaResource{}
var _ resource.ResourceWithModifyPlan = &SchemaResource{}

// SchemaResource loads a bundled naming convention, merges an optional
// partial override and runtime parameters on top of it, and persists a
// random unique id in state - all in one resource, since the id must stay
// stable across plans/applies and provider-defined functions cannot hold
// state (they must return the same result for the same arguments on every
// call).
type SchemaResource struct{}

func NewSchemaResource() resource.Resource {
	return &SchemaResource{}
}

type schemaResourceModel struct {
	Convention types.String  `tfsdk:"convention"`
	Naming     types.String  `tfsdk:"naming"`
	Parameters types.Dynamic `tfsdk:"parameters"`
	RandomSeed types.String  `tfsdk:"random_seed"`

	IndexModifier     types.Dynamic `tfsdk:"index_modifier"`
	EnforceLowerCase  types.Dynamic `tfsdk:"enforce_lower_case"`
	Abbreviations     types.Dynamic `tfsdk:"abbreviations"`
	Mappings          types.Dynamic `tfsdk:"mappings"`
	Patterns          types.Dynamic `tfsdk:"patterns"`
	DefaultParameters types.Dynamic `tfsdk:"default_parameters"`

	Random types.Object `tfsdk:"random"`
	ID     types.String `tfsdk:"id"`
}

var randomAttributeTypes = map[string]attr.Type{
	"uuid": types.StringType,
}

func (r *SchemaResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schema"
}

func (r *SchemaResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Resolves a naming schema from a bundled convention, an optional partial override, and runtime parameters - and persists a random unique id for use in generated names (e.g. the `<UNIQUE_ID_n>` pattern token).",
		Attributes: map[string]schema.Attribute{
			"convention": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("default"),
				MarkdownDescription: "Name of the bundled convention to load (e.g. `\"default\"`). Defaults to `\"default\"`.",
			},
			"naming": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Path to a custom YAML file that partially overrides the bundled convention (e.g. `\"${path.module}/naming.yaml\"`). Any key left unset in it falls back to the bundled convention, except `patterns`, which is replaced wholesale when set.",
			},
			"parameters": schema.DynamicAttribute{
				Optional:            true,
				MarkdownDescription: "Runtime parameters (`location`, `environment`, `name`, ...) used when rendering names.",
			},
			"random_seed": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional seed for the generated unique id. The same seed always produces the same id (a deterministic UUID derived from it). If unset, a random id is generated once on create and kept stable across updates.",
			},
			"index_modifier": schema.DynamicAttribute{
				Computed:            true,
				MarkdownDescription: "The resolved index offset (e.g. `1` so indices start at `01`).",
			},
			"enforce_lower_case": schema.DynamicAttribute{
				Computed:            true,
				MarkdownDescription: "The resolved lower-casing settings.",
			},
			"abbreviations": schema.DynamicAttribute{
				Computed:            true,
				MarkdownDescription: "The merged abbreviations table.",
			},
			"mappings": schema.DynamicAttribute{
				Computed:            true,
				MarkdownDescription: "The merged mappings table.",
			},
			"patterns": schema.DynamicAttribute{
				Computed:            true,
				MarkdownDescription: "The resolved patterns table.",
			},
			"default_parameters": schema.DynamicAttribute{
				Computed:            true,
				MarkdownDescription: "Normalized copy of `parameters` (an empty object if `parameters` was left unset).",
			},
			"random": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "A random source for generating unique naming ids. Generated once and kept stable across updates.",
				Attributes: map[string]schema.Attribute{
					"uuid": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "A random UUID, generated once when the resource is created.",
					},
				},
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Same value as `random.uuid`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// ModifyPlan pre-computes the derived schema attributes at plan time
// instead of leaving them "(known after apply)". Resolving the schema is a
// pure, local operation (embedded YAML + in-memory merging), so it is safe
// to run during planning whenever the inputs are already known.
func (r *SchemaResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		// The resource is being destroyed; nothing to compute.
		return
	}

	var plan schemaResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Convention.IsUnknown() || plan.Naming.IsUnknown() || plan.Parameters.IsUnknown() {
		// Inputs depend on a value only known after apply; leave the
		// computed attributes as Terraform already planned them.
		return
	}

	if resp.Diagnostics.Append(resolveSchema(ctx, &plan)...); resp.Diagnostics.HasError() {
		return
	}

	// A known, non-empty random_seed deterministically pins the id, so it
	// can be computed accurately at plan time too - overriding whatever
	// objectplanmodifier.UseStateForUnknown already carried forward. An
	// unset seed leaves the carried-forward (or, on create, still unknown)
	// value untouched; Create fills it in with a fresh random id.
	if !plan.RandomSeed.IsUnknown() && !plan.RandomSeed.IsNull() && plan.RandomSeed.ValueString() != "" {
		id := deterministicUUID(plan.RandomSeed.ValueString())
		randomObj, diags := types.ObjectValue(randomAttributeTypes, map[string]attr.Value{
			"uuid": types.StringValue(id),
		})
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		plan.Random = randomObj
		plan.ID = types.StringValue(id)
	}

	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

func (r *SchemaResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data schemaResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if resp.Diagnostics.Append(resolveSchema(ctx, &data)...); resp.Diagnostics.HasError() {
		return
	}

	// ModifyPlan already fills these in deterministically when random_seed
	// is set. Only a still-unknown value (unseeded create) needs a fresh
	// random id here.
	if data.Random.IsUnknown() {
		var id string
		if seed := data.RandomSeed.ValueString(); seed != "" {
			id = deterministicUUID(seed)
		} else {
			id = uuid.New().String()
		}

		randomObj, diags := types.ObjectValue(randomAttributeTypes, map[string]attr.Value{
			"uuid": types.StringValue(id),
		})
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.Random = randomObj
		data.ID = types.StringValue(id)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SchemaResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data schemaResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if resp.Diagnostics.Append(resolveSchema(ctx, &data)...); resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SchemaResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data schemaResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// data.Random and data.ID are already carried over from prior state by
	// their UseStateForUnknown plan modifiers - only the derived schema
	// fields need recomputing here.
	if resp.Diagnostics.Append(resolveSchema(ctx, &data)...); resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SchemaResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
}

// resolveSchema loads the convention, merges naming/parameters on top of
// it via schemaengine.Resolve, and writes the resulting computed attributes
// into data.
func resolveSchema(ctx context.Context, data *schemaResourceModel) (diags diag.Diagnostics) {
	// 1. Load the user-provided override file (if any). Empty path -> nil map.
	namingMap, err := schemaengine.LoadOverrideFile(data.Naming.ValueString())
	if err != nil {
		diags.AddAttributeError(path.Root("naming"), "Invalid naming file", err.Error())
		return diags
	}

	// 2. Load the bundled convention (e.g. "default") that ships embedded
	// in the provider binary. This is the base everything else merges onto.
	conventionMap, err := schemaengine.LoadConvention(data.Convention.ValueString())
	if err != nil {
		diags.AddAttributeError(path.Root("convention"), "Invalid convention", err.Error())
		return diags
	}

	// 3. Merge the user's naming override onto the bundled convention.
	//    - abbreviations/mappings: deep merge, override wins on conflicts,
	//      untouched nested keys fall back to the convention.
	//    - patterns: wholesale replace when set, otherwise fall back to the
	//      convention's patterns entirely (no merging).
	//    - index_modifier/enforce_lower_case: override wins if set, else
	//      fall back to the convention's value.
	abbreviations := schemaengine.DeepMergeUserWins(
		schemaengine.AsMap(schemaengine.GetKey(conventionMap, "abbreviations")),
		schemaengine.AsMap(schemaengine.GetKey(namingMap, "abbreviations")),
	)
	mappings := schemaengine.Coalesce(
		schemaengine.GetKey(namingMap, "mappings"),
		schemaengine.GetKey(conventionMap, "mappings"),
	)
	patterns := schemaengine.Coalesce(
		schemaengine.GetKey(namingMap, "patterns"),
		schemaengine.GetKey(conventionMap, "patterns"),
	)
	indexModifier := schemaengine.Coalesce(
		schemaengine.GetKey(namingMap, "index_modifier"),
		schemaengine.GetKey(conventionMap, "index_modifier"),
	)
	enforceLowerCase := schemaengine.Coalesce(
		schemaengine.GetKey(namingMap, "enforce_lower_case"),
		schemaengine.GetKey(conventionMap, "enforce_lower_case"),
	)

	// 4. default_parameters = parameters, but an empty object instead of
	//    null when unset. No conversion needed - parameters is already a
	//    valid Dynamic value, just pass it through.
	defaultParameters := data.Parameters
	if defaultParameters.IsNull() {
		defaultParameters = types.DynamicValue(types.ObjectValueMust(
			map[string]attr.Type{}, map[string]attr.Value{},
		))
	}

	// 5. Write everything into the resource's computed attributes - this is
	//    what shows up as naming_schema.this.abbreviations etc., the same
	//    role the old module's `output` blocks played.
	set := func(key string, value interface{}, target *types.Dynamic) {
		dyn, err := schemaengine.ToDynamic(value)
		if err != nil {
			diags.AddError("Failed to encode "+key, err.Error())
			return
		}
		*target = dyn
	}

	set("index_modifier", indexModifier, &data.IndexModifier)
	set("enforce_lower_case", enforceLowerCase, &data.EnforceLowerCase)
	set("abbreviations", abbreviations, &data.Abbreviations)
	set("mappings", mappings, &data.Mappings)
	set("patterns", patterns, &data.Patterns)
	// default_parameters is already a Dynamic value (see step 4) - no
	// ToDynamic conversion needed, just assign it directly.
	data.DefaultParameters = defaultParameters
	set("default_parameters", defaultParameters, &data.DefaultParameters)

	return diags
}
