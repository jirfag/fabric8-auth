package graph

import (
	"context"

	resource "github.com/fabric8-services/fabric8-auth/authorization/resource/repository"
	resourcetype "github.com/fabric8-services/fabric8-auth/authorization/resourcetype/repository"
	role "github.com/fabric8-services/fabric8-auth/authorization/role/repository"

	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
)

// resourceWrapper represents a resource domain object
type resourceWrapper struct {
	baseWrapper
	resource *resource.Resource
}

func newResourceWrapper(g *TestGraph, params []interface{}) interface{} {
	w := resourceWrapper{baseWrapper: baseWrapper{g}}

	var resourceName *string
	var resourceType *resourcetype.ResourceType
	var parentResource *resource.Resource

	for i := range params {
		switch t := params[i].(type) {
		case string:
			resourceName = &t
		case *string:
			resourceName = t
		case resourceTypeWrapper:
			resourceType = t.resourceType
		case *resourceTypeWrapper:
			resourceType = t.resourceType
		case resourceWrapper:
			parentResource = t.Resource()
		case *resourceWrapper:
			parentResource = t.Resource()
		case spaceWrapper:
			parentResource = t.Resource()
		case *spaceWrapper:
			parentResource = t.Resource()
		case organizationWrapper:
			parentResource = t.Resource()
		case *organizationWrapper:
			parentResource = t.Resource()
		case resource.Resource:
			parentResource = &t
		case *resource.Resource:
			parentResource = t
		}
	}

	if resourceType == nil {
		resourceType = w.graph.CreateResourceType().ResourceType()
		g.t.Logf("created new resource type with name='%v' (%v)", resourceType.Name, resourceType.ResourceTypeID)
	}

	if resourceName == nil {
		nm := "Resource-" + uuid.NewV4().String()
		resourceName = &nm
	}

	w.resource = &resource.Resource{
		Name:           *resourceName,
		ResourceTypeID: resourceType.ResourceTypeID,
	}

	if parentResource != nil {
		w.resource.ParentResourceID = &parentResource.ResourceID
	}

	err := g.app.ResourceRepository().Create(g.ctx, w.resource)
	require.NoError(g.t, err)

	w.resource.ParentResource = parentResource
	w.resource.ResourceType = *resourceType
	return &w
}

func loadResourceWrapper(g *TestGraph, resourceID string) resourceWrapper {
	w := resourceWrapper{baseWrapper: baseWrapper{g}}

	var native resource.Resource
	err := w.graph.db.Table("resource").Where("resource_id = ?", resourceID).Find(&native).Error
	require.NoError(w.graph.t, err)

	w.resource = &native

	return w
}

func (w *resourceWrapper) Resource() *resource.Resource {
	return w.resource
}

func (w *resourceWrapper) ResourceID() string {
	return w.resource.ResourceID
}

// AddRole assigns the given role to a user for the space
func (w *resourceWrapper) AddRole(wrapper interface{}, roleWrapper *roleWrapper) *resourceWrapper {
	addRole(w.baseWrapper, w.resource, w.resource.ResourceType.Name, identityIDFromWrapper(w.graph.t, wrapper), roleWrapper.Role())
	return w
}

func addRoleByName(w baseWrapper, resource *resource.Resource, resourceTypeName string, identityID uuid.UUID, roleName string) {
	r, err := w.graph.app.RoleRepository().Lookup(w.graph.ctx, roleName, resourceTypeName)
	require.NoError(w.graph.t, err)
	identityRole := &role.IdentityRole{
		ResourceID: resource.ResourceID,
		IdentityID: identityID,
		RoleID:     r.RoleID,
	}
	err = w.graph.app.IdentityRoleRepository().Create(w.graph.ctx, identityRole)
	require.NoError(w.graph.t, err)
}

func removeRoleByName(w baseWrapper, resource *resource.Resource, resourceTypeName string, identityID uuid.UUID, roleName string) {
	roles, err := w.graph.app.IdentityRoleRepository().FindIdentityRolesByIdentityAndResource(w.graph.ctx, resource.ResourceID, identityID)
	require.NoError(w.graph.t, err)
	for _, r := range roles {
		role, err := w.graph.app.RoleRepository().Load(w.graph.ctx, r.RoleID)
		require.NoError(w.graph.t, err)
		if role.Name == roleName {
			w.graph.app.IdentityRoleRepository().Delete(context.Background(), r.IdentityRoleID)
			return
		}
	}
	w.graph.t.Fatalf("unable to remove role '%s' for user with identity '%v' on resource '%s'", roleName, identityID, resource.ResourceID)
}

func addRole(w baseWrapper, res *resource.Resource, resourceTypeName string, identityID uuid.UUID, r *role.Role) {
	// check that the role applies to the given resource type
	require.NotNil(w.graph.t, r)
	r, err := w.graph.app.RoleRepository().Load(w.graph.ctx, r.RoleID)
	require.NoError(w.graph.t, err)
	require.NotNil(w.graph.t, r.ResourceType)
	require.Equal(w.graph.t, r.ResourceType.Name, resourceTypeName, "role does not apply to resources of type '%s' but to '%s'", resourceTypeName, r.ResourceType.Name)
	identityRole := &role.IdentityRole{
		ResourceID: res.ResourceID,
		IdentityID: identityID,
		RoleID:     r.RoleID,
	}
	err = w.graph.app.IdentityRoleRepository().Create(w.graph.ctx, identityRole)
	require.NoError(w.graph.t, err)
}
