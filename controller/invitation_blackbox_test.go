package controller_test

import (
	"testing"

	account "github.com/fabric8-services/fabric8-auth/account/repository"
	"github.com/fabric8-services/fabric8-auth/app"
	"github.com/fabric8-services/fabric8-auth/app/test"
	. "github.com/fabric8-services/fabric8-auth/controller"
	"github.com/fabric8-services/fabric8-auth/gormtestsupport"

	testsupport "github.com/fabric8-services/fabric8-auth/test"

	"github.com/goadesign/goa"

	"net/url"

	"github.com/fabric8-services/fabric8-auth/application/service"
	"github.com/fabric8-services/fabric8-auth/authorization"
	invitationrepo "github.com/fabric8-services/fabric8-auth/authorization/invitation/repository"
	"github.com/fabric8-services/fabric8-auth/errors"
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestInvitationController(t *testing.T) {
	suite.Run(t, &InvitationControllerTestSuite{DBTestSuite: gormtestsupport.NewDBTestSuite()})
}

type InvitationControllerTestSuite struct {
	gormtestsupport.DBTestSuite
	testIdentity account.Identity
	service      *goa.Service
	invService   service.InvitationService
	invRepo      invitationrepo.InvitationRepository
}

func (s *InvitationControllerTestSuite) SetupSuite() {
	s.DBTestSuite.SetupSuite()
	s.invService = s.Application.InvitationService()
	s.invRepo = invitationrepo.NewInvitationRepository(s.DB)

	var err error
	s.testIdentity, err = testsupport.CreateTestIdentity(s.DB,
		"InvitationCreatorUser-"+uuid.NewV4().String(),
		"TestInvitation")
	require.Nil(s.T(), err)
}

func (s *InvitationControllerTestSuite) SecuredController(identity account.Identity) (*goa.Service, *InvitationController) {
	svc := testsupport.ServiceAsUser("Invitation-Service", identity)
	return svc, NewInvitationController(svc, s.Application, s.Configuration)
}

func (s *InvitationControllerTestSuite) UnsecuredController() (*goa.Service, *InvitationController) {
	svc := goa.New("Invitation-Service")
	controller := NewInvitationController(svc, s.Application, s.Configuration)
	return svc, controller
}

/*
* This test will attempt to create a new invitation for a user to become a member of an organization
 */
func (s *InvitationControllerTestSuite) TestCreateInvitation() {

	s.T().Run("team member", func(t *testing.T) {

		t.Run("success", func(t *testing.T) {
			// given
			g := s.NewTestGraph(t)
			team := g.CreateTeam()
			r := g.CreateRole(g.LoadResourceType(authorization.IdentityResourceTypeTeam))
			r.AddScope(authorization.ManageTeamMembersScope)
			team.AssignRole(&s.testIdentity, r.Role())
			invitee := g.CreateUser()
			inviteeID := invitee.IdentityID().String()
			payload := newCreateInvitationPayload(inviteeID, true)
			service, controller := s.SecuredController(s.testIdentity)
			// when
			test.CreateInviteInvitationCreated(t, service.Context, service, controller, team.TeamID().String(), payload)
			// then
			invitations, err := s.invRepo.ListForIdentity(s.Ctx, team.TeamID())
			require.NoError(t, err, "could not list invitations")
			// We should have 1 invitation
			require.Len(t, invitations, 1)
			assert.Equal(t, invitee.IdentityID(), invitations[0].IdentityID)
			assert.True(t, invitations[0].Member)
		})
	})

	s.T().Run("organization", func(t *testing.T) {

		t.Run("success", func(t *testing.T) {
			// given
			g := s.NewTestGraph(t)
			team := g.CreateTeam()
			r := g.CreateRole(g.LoadResourceType(authorization.IdentityResourceTypeTeam))
			r.AddScope(authorization.ManageTeamMembersScope)
			team.AssignRole(&s.testIdentity, r.Role())
			invitee := g.CreateUser()
			inviteeID := invitee.IdentityID().String()
			payload := newCreateInvitationPayload(inviteeID, false, r.Role().Name)
			service, controller := s.SecuredController(s.testIdentity)
			// when
			test.CreateInviteInvitationCreated(t, service.Context, service, controller, team.TeamID().String(), payload)
			// then
			invitations, err := s.invRepo.ListForIdentity(s.Ctx, team.TeamID())
			require.NoError(t, err, "could not list invitations")
			// We should have 1 invitation
			require.Len(t, invitations, 1)
			assert.Equal(t, invitee.IdentityID(), invitations[0].IdentityID)
			assert.False(t, invitations[0].Member)
			roles, err := s.invRepo.ListRoles(s.Ctx, invitations[0].InvitationID)
			require.NoError(t, err, "could not list invitation roles")
			// We should have 1 role
			require.Len(t, roles, 1)
			// And it should be the owner role
			assert.Equal(t, r.Role().Name, roles[0].Name)
		})

		t.Run("unauthorized", func(t *testing.T) { // This test will attempt to create a new invitation for a user to become a member of an organization, however perform an unauthorized request to create the invitation
			// given
			orgIdentity, err := testsupport.CreateTestOrganization(s.Ctx, s.DB, s.Application, s.testIdentity.ID, "Acme Corporation"+uuid.NewV4().String())
			require.NoError(t, err, "could not create organization")
			testUsername := "jsmith" + uuid.NewV4().String()
			invitee, err := testsupport.CreateTestIdentityAndUser(s.DB, testUsername, "InvitationTest")
			require.NoError(t, err, "could not create invitee user")
			inviteeID := invitee.ID.String()
			payload := newCreateInvitationPayload(inviteeID, true)
			service, controller := s.UnsecuredController()
			// when
			test.CreateInviteInvitationUnauthorized(t, service.Context, service, controller, orgIdentity.ID.String(), payload)
			// then
			invitations, err := s.invRepo.ListForIdentity(s.Ctx, orgIdentity.ID)
			require.NoError(t, err, "could not list invitations")
			// We should have no invitations
			assert.Empty(t, invitations)
		})

		t.Run("invalid role", func(t *testing.T) { // This test will attempt to create a new invitation for a user to accept an invalid role in an organization, we should get a bad request error as a result
			// given
			orgIdentity, err := testsupport.CreateTestOrganization(s.Ctx, s.DB, s.Application, s.testIdentity.ID, "Acme Corporation"+uuid.NewV4().String())
			require.NoError(t, err, "could not create organization")
			testUsername := "jsmith" + uuid.NewV4().String()
			invitee, err := testsupport.CreateTestIdentityAndUser(s.DB, testUsername, "InvitationTest")
			require.NoError(t, err, "could not create invitee user")
			inviteeID := invitee.ID.String()
			payload := newCreateInvitationPayload(inviteeID, false, "foobar")
			service, controller := s.SecuredController(s.testIdentity)
			// when
			test.CreateInviteInvitationBadRequest(t, service.Context, service, controller, orgIdentity.ID.String(), payload)
			invitations, err := s.invRepo.ListForIdentity(s.Ctx, orgIdentity.ID)
			require.NoError(t, err, "could not list invitations")
			// We should have no invitations
			assert.Empty(t, invitations)
		})

		t.Run("invalid user", func(t *testing.T) { // This test will attempt to create a new invitation however provide no identifying information for the user we should get a bad request error as a result
			// given
			orgIdentity, err := testsupport.CreateTestOrganization(s.Ctx, s.DB, s.Application, s.testIdentity.ID, "Acme Corporation"+uuid.NewV4().String())
			require.NoError(t, err, "could not create organization")
			service, controller := s.SecuredController(s.testIdentity)
			payload := newCreateInvitationPayload("", true, "foobar")
			// when
			test.CreateInviteInvitationBadRequest(t, service.Context, service, controller, orgIdentity.ID.String(), payload)
			// then
			invitations, err := s.invRepo.ListForIdentity(s.Ctx, orgIdentity.ID)
			require.NoError(t, err, "could not list invitations")
			// We should have no invitations
			assert.Empty(t, invitations)
		})
	})

}

func (s *InvitationControllerTestSuite) TestAcceptInvitation() {

	s.T().Run("ok", func(t *testing.T) {
		// given
		g := s.NewTestGraph(t)
		team := g.CreateTeam()
		invitee := g.CreateUser()
		inv := g.CreateInvitation(team, invitee)
		service, controller := s.UnsecuredController()
		// when
		response := test.AcceptInviteInvitationTemporaryRedirect(t, service.Context, service, controller, inv.Invitation().AcceptCode.String())
		// then
		require.NotNil(t, response.Header().Get("Location"))
		// The invitation should no longer be there after acceptance
		_, err := s.Application.InvitationRepository().FindByAcceptCode(s.Ctx, inv.Invitation().AcceptCode)
		require.Error(t, err)
		require.IsType(t, errors.NotFoundError{}, err)
	})

	s.T().Run("failure", func(t *testing.T) {

		s.T().Run("non-uuid code", func(t *testing.T) {
			// given
			g := s.NewTestGraph(t)
			team := g.CreateTeam()
			invitee := g.CreateUser()
			g.CreateInvitation(team, invitee)
			service, controller := s.SecuredController(s.testIdentity)
			// when
			response := test.AcceptInviteInvitationTemporaryRedirect(t, service.Context, service, controller, "foo")
			// then
			parsedURL, err := url.Parse(response.Header().Get("Location"))
			require.NoError(t, err)
			parameters := parsedURL.Query()
			require.NotNil(t, parameters.Get("error"))
		})

		s.T().Run("invalid code", func(t *testing.T) {
			// given
			service, controller := s.SecuredController(s.testIdentity)
			// when
			// This should still work, however there should now be an error param in the redirect URL
			response := test.AcceptInviteInvitationTemporaryRedirect(t, service.Context, service, controller, uuid.NewV4().String())
			// then
			require.NotNil(t, response.Header().Get("Location"))
			parsedURL, err := url.Parse(response.Header().Get("Location"))
			require.NoError(t, err)
			parameters := parsedURL.Query()
			require.NotNil(t, parameters.Get("error"))
		})
	})

}

func (s *InvitationControllerTestSuite) TestRescindInvitation() {

	s.T().Run("ok", func(t *testing.T) {
		// given
		g := s.NewTestGraph(t)
		team := g.CreateTeam()
		invitee := g.CreateUser()
		inv := g.CreateInvitation(team, invitee)
		r := g.CreateRole(g.LoadResourceType(authorization.IdentityResourceTypeTeam))
		r.AddScope(authorization.ManageTeamMembersScope)
		team.AssignRole(&s.testIdentity, r.Role())
		service, controller := s.SecuredController(s.testIdentity)
		// when
		response := test.RescindInviteInvitationOK(t, service.Context, service, controller, inv.Invitation().InvitationID.String())
		// then
		require.NotNil(t, response.Header().Get("Location"))
		// The invitation should no longer be there after rescinding
		_, err := s.Application.InvitationRepository().Load(s.Ctx, inv.Invitation().InvitationID)
		require.Error(t, err)
		require.IsType(t, errors.NotFoundError{}, err)
	})

	s.T().Run("fail", func(t *testing.T) {
		t.Run("invalid id", func(t *testing.T) {
			// given
			g := s.NewTestGraph(t)
			team := g.CreateTeam()
			invitee := g.CreateUser()
			g.CreateInvitation(team, invitee)
			r := g.CreateRole(g.LoadResourceType(authorization.IdentityResourceTypeTeam))
			r.AddScope(authorization.ManageTeamMembersScope)
			team.AssignRole(&s.testIdentity, r.Role())
			service, controller := s.SecuredController(s.testIdentity)
			// when
			response, _ := test.RescindInviteInvitationNotFound(t, service.Context, service, controller, uuid.NewV4().String())
			// then
			require.NotNil(t, response.Header().Get("Location"))
		})

		t.Run("non-uuid id", func(t *testing.T) {
			// given
			g := s.NewTestGraph(t)
			team := g.CreateTeam()
			invitee := g.CreateUser()
			g.CreateInvitation(team, invitee)
			r := g.CreateRole(g.LoadResourceType(authorization.IdentityResourceTypeTeam))
			r.AddScope(authorization.ManageTeamMembersScope)
			team.AssignRole(&s.testIdentity, r.Role())
			service, controller := s.SecuredController(s.testIdentity)
			// when
			response, _ := test.RescindInviteInvitationNotFound(t, service.Context, service, controller, "foo")
			// then
			require.NotNil(t, response.Header().Get("Location"))
		})
	})

	s.T().Run("unauthorized", func(t *testing.T) {
		// given
		g := s.NewTestGraph(t)
		team := g.CreateTeam()
		invitee := g.CreateUser()
		inv := g.CreateInvitation(team, invitee)
		service, controller := s.SecuredController(s.testIdentity)
		// when
		response, _ := test.RescindInviteInvitationInternalServerError(t, service.Context, service, controller, inv.Invitation().InvitationID.String())
		// then
		require.NotNil(t, response.Header().Get("Location"))
		_, err := s.Application.InvitationRepository().Load(s.Ctx, inv.Invitation().InvitationID)
		require.NoError(t, err)
	})
}

func newCreateInvitationPayload(inviteeID string, member bool, roles ...string) *app.CreateInviteInvitationPayload {
	return &app.CreateInviteInvitationPayload{
		Data: []*app.Invitee{
			{
				IdentityID: &inviteeID,
				Member:     &member,
				Roles:      roles,
			},
		},
	}
}
