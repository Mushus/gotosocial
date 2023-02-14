package federation

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/superseriousbusiness/activity/streams"
	"github.com/superseriousbusiness/gotosocial/internal/db"
	"github.com/superseriousbusiness/gotosocial/internal/gtserror"
	"github.com/superseriousbusiness/gotosocial/internal/transport"
)

// FediFollowersGet handles the getting of a fedi/activitypub representation of a user/account's followers, performing appropriate
// authentication before returning a JSON serializable interface to the caller.
func (p *FederationProcessor) FediFollowersGet(ctx context.Context, requestedUsername string, requestURL *url.URL) (interface{}, gtserror.WithCode) {
	// get the account the request is referring to
	requestedAccount, err := p.db.GetAccountByUsernameDomain(ctx, requestedUsername, "")
	if err != nil {
		return nil, gtserror.NewErrorNotFound(fmt.Errorf("database error getting account with username %s: %s", requestedUsername, err))
	}

	// authenticate the request
	requestingAccountURI, errWithCode := p.federator.AuthenticateFederatedRequest(ctx, requestedUsername)
	if errWithCode != nil {
		return nil, errWithCode
	}

	requestingAccount, err := p.federator.GetAccountByURI(
		transport.WithFastfail(ctx), requestedUsername, requestingAccountURI, false,
	)
	if err != nil {
		return nil, gtserror.NewErrorUnauthorized(err)
	}

	blocked, err := p.db.IsBlocked(ctx, requestedAccount.ID, requestingAccount.ID, true)
	if err != nil {
		return nil, gtserror.NewErrorInternalError(err)
	}

	if blocked {
		return nil, gtserror.NewErrorUnauthorized(fmt.Errorf("block exists between accounts %s and %s", requestedAccount.ID, requestingAccount.ID))
	}

	requestedAccountURI, err := url.Parse(requestedAccount.URI)
	if err != nil {
		return nil, gtserror.NewErrorInternalError(fmt.Errorf("error parsing url %s: %s", requestedAccount.URI, err))
	}

	requestedFollowers, err := p.federator.FederatingDB().Followers(ctx, requestedAccountURI)
	if err != nil {
		return nil, gtserror.NewErrorInternalError(fmt.Errorf("error fetching followers for uri %s: %s", requestedAccountURI.String(), err))
	}

	data, err := streams.Serialize(requestedFollowers)
	if err != nil {
		return nil, gtserror.NewErrorInternalError(err)
	}

	return data, nil
}

// FediFollowingGet handles the getting of a fedi/activitypub representation of a user/account's following, performing appropriate
// authentication before returning a JSON serializable interface to the caller.
func (p *FederationProcessor) FediFollowingGet(ctx context.Context, requestedUsername string, requestURL *url.URL) (interface{}, gtserror.WithCode) {
	// get the account the request is referring to
	requestedAccount, err := p.db.GetAccountByUsernameDomain(ctx, requestedUsername, "")
	if err != nil {
		return nil, gtserror.NewErrorNotFound(fmt.Errorf("database error getting account with username %s: %s", requestedUsername, err))
	}

	// authenticate the request
	requestingAccountURI, errWithCode := p.federator.AuthenticateFederatedRequest(ctx, requestedUsername)
	if errWithCode != nil {
		return nil, errWithCode
	}

	requestingAccount, err := p.federator.GetAccountByURI(
		transport.WithFastfail(ctx), requestedUsername, requestingAccountURI, false,
	)
	if err != nil {
		return nil, gtserror.NewErrorUnauthorized(err)
	}

	blocked, err := p.db.IsBlocked(ctx, requestedAccount.ID, requestingAccount.ID, true)
	if err != nil {
		return nil, gtserror.NewErrorInternalError(err)
	}

	if blocked {
		return nil, gtserror.NewErrorUnauthorized(fmt.Errorf("block exists between accounts %s and %s", requestedAccount.ID, requestingAccount.ID))
	}

	requestedAccountURI, err := url.Parse(requestedAccount.URI)
	if err != nil {
		return nil, gtserror.NewErrorInternalError(fmt.Errorf("error parsing url %s: %s", requestedAccount.URI, err))
	}

	requestedFollowing, err := p.federator.FederatingDB().Following(ctx, requestedAccountURI)
	if err != nil {
		return nil, gtserror.NewErrorInternalError(fmt.Errorf("error fetching following for uri %s: %s", requestedAccountURI.String(), err))
	}

	data, err := streams.Serialize(requestedFollowing)
	if err != nil {
		return nil, gtserror.NewErrorInternalError(err)
	}

	return data, nil
}

// FediOutboxGet returns the activitypub representation of a local user's outbox.
// This contains links to PUBLIC posts made by this user.
func (p *FederationProcessor) FediOutboxGet(ctx context.Context, requestedUsername string, page bool, maxID string, minID string, requestURL *url.URL) (interface{}, gtserror.WithCode) {
	// get the account the request is referring to
	requestedAccount, err := p.db.GetAccountByUsernameDomain(ctx, requestedUsername, "")
	if err != nil {
		return nil, gtserror.NewErrorNotFound(fmt.Errorf("database error getting account with username %s: %s", requestedUsername, err))
	}

	// authenticate the request
	requestingAccountURI, errWithCode := p.federator.AuthenticateFederatedRequest(ctx, requestedUsername)
	if errWithCode != nil {
		return nil, errWithCode
	}

	requestingAccount, err := p.federator.GetAccountByURI(
		transport.WithFastfail(ctx), requestedUsername, requestingAccountURI, false,
	)
	if err != nil {
		return nil, gtserror.NewErrorUnauthorized(err)
	}

	// authorize the request:
	// 1. check if a block exists between the requester and the requestee
	blocked, err := p.db.IsBlocked(ctx, requestedAccount.ID, requestingAccount.ID, true)
	if err != nil {
		return nil, gtserror.NewErrorInternalError(err)
	}
	if blocked {
		return nil, gtserror.NewErrorUnauthorized(fmt.Errorf("block exists between accounts %s and %s", requestedAccount.ID, requestingAccount.ID))
	}

	var data map[string]interface{}
	// now there are two scenarios:
	// 1. we're asked for the whole collection and not a page -- we can just return the collection, with no items, but a link to 'first' page.
	// 2. we're asked for a specific page; this can be either the first page or any other page

	if !page {
		/*
			scenario 1: return the collection with no items
			we want something that looks like this:
			{
				"@context": "https://www.w3.org/ns/activitystreams",
				"id": "https://example.org/users/whatever/outbox",
				"type": "OrderedCollection",
				"first": "https://example.org/users/whatever/outbox?page=true",
				"last": "https://example.org/users/whatever/outbox?min_id=0&page=true"
			}
		*/
		collection, err := p.tc.OutboxToASCollection(ctx, requestedAccount.OutboxURI)
		if err != nil {
			return nil, gtserror.NewErrorInternalError(err)
		}

		data, err = streams.Serialize(collection)
		if err != nil {
			return nil, gtserror.NewErrorInternalError(err)
		}

		return data, nil
	}

	// scenario 2 -- get the requested page
	// limit pages to 30 entries per page
	publicStatuses, err := p.db.GetAccountStatuses(ctx, requestedAccount.ID, 30, true, true, maxID, minID, false, false, true)
	if err != nil && err != db.ErrNoEntries {
		return nil, gtserror.NewErrorInternalError(err)
	}

	outboxPage, err := p.tc.StatusesToASOutboxPage(ctx, requestedAccount.OutboxURI, maxID, minID, publicStatuses)
	if err != nil {
		return nil, gtserror.NewErrorInternalError(err)
	}
	data, err = streams.Serialize(outboxPage)
	if err != nil {
		return nil, gtserror.NewErrorInternalError(err)
	}

	return data, nil
}

// FediInboxPost handles POST requests to a user's inbox for new activitypub messages.
//
// FediInboxPost returns true if the request was handled as an ActivityPub POST to an actor's inbox.
// If false, the request was not an ActivityPub request and may still be handled by the caller in another way, such as serving a web page.
//
// If the error is nil, then the ResponseWriter's headers and response has already been written. If a non-nil error is returned, then no response has been written.
//
// If the Actor was constructed with the Federated Protocol enabled, side effects will occur.
//
// If the Federated Protocol is not enabled, writes the http.StatusMethodNotAllowed status code in the response. No side effects occur.
func (p *FederationProcessor) FediInboxPost(ctx context.Context, w http.ResponseWriter, r *http.Request) (bool, error) {
	return p.federator.FederatingActor().PostInbox(ctx, w, r)
}
