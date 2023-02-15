/*
   GoToSocial
   Copyright (C) 2021-2023 GoToSocial Authors admin@gotosocial.org

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published by
   the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package status

import (
	"context"

	apimodel "github.com/superseriousbusiness/gotosocial/internal/api/model"
	"github.com/superseriousbusiness/gotosocial/internal/concurrency"
	"github.com/superseriousbusiness/gotosocial/internal/db"
	"github.com/superseriousbusiness/gotosocial/internal/gtserror"
	"github.com/superseriousbusiness/gotosocial/internal/gtsmodel"
	"github.com/superseriousbusiness/gotosocial/internal/messages"
	"github.com/superseriousbusiness/gotosocial/internal/text"
	"github.com/superseriousbusiness/gotosocial/internal/typeutils"
	"github.com/superseriousbusiness/gotosocial/internal/visibility"
)

// Processor wraps a bunch of functions for processing statuses.
type rocessor interface {
	
	Create(ctx context.Context, account *gtsmodel.Account, application *gtsmodel.Application, form *apimodel.AdvancedStatusCreateForm) (*apimodel.Status, gtserror.WithCode)
	
	Delete(ctx context.Context, account *gtsmodel.Account, targetStatusID string) (*apimodel.Status, gtserror.WithCode)
	
	Fave(ctx context.Context, account *gtsmodel.Account, targetStatusID string) (*apimodel.Status, gtserror.WithCode)
	
	Boost(ctx context.Context, account *gtsmodel.Account, application *gtsmodel.Application, targetStatusID string) (*apimodel.Status, gtserror.WithCode)
	
	Unboost(ctx context.Context, account *gtsmodel.Account, application *gtsmodel.Application, targetStatusID string) (*apimodel.Status, gtserror.WithCode)
	
	BoostedBy(ctx context.Context, account *gtsmodel.Account, targetStatusID string) ([]*apimodel.Account, gtserror.WithCode)
	
	FavedBy(ctx context.Context, account *gtsmodel.Account, targetStatusID string) ([]*apimodel.Account, gtserror.WithCode)
	
	Get(ctx context.Context, account *gtsmodel.Account, targetStatusID string) (*apimodel.Status, gtserror.WithCode)
	
	Unfave(ctx context.Context, account *gtsmodel.Account, targetStatusID string) (*apimodel.Status, gtserror.WithCode)
	
	Context(ctx context.Context, account *gtsmodel.Account, targetStatusID string) (*apimodel.Context, gtserror.WithCode)
	
	Bookmark(ctx context.Context, account *gtsmodel.Account, targetStatusID string) (*apimodel.Status, gtserror.WithCode)
	
	Unbookmark(ctx context.Context, account *gtsmodel.Account, targetStatusID string) (*apimodel.Status, gtserror.WithCode)
}

type StatusProcessor struct { //nolint:revive
	tc           typeutils.TypeConverter
	db           db.DB
	filter       visibility.Filter
	formatter    text.Formatter
	clientWorker *concurrency.WorkerPool[messages.FromClientAPI]
	parseMention gtsmodel.ParseMentionFunc
}

// New returns a new status processor.
func New(db db.DB, tc typeutils.TypeConverter, clientWorker *concurrency.WorkerPool[messages.FromClientAPI], parseMention gtsmodel.ParseMentionFunc) StatusProcessor {
	return StatusProcessor{
		tc:           tc,
		db:           db,
		filter:       visibility.NewFilter(db),
		formatter:    text.NewFormatter(db),
		clientWorker: clientWorker,
		parseMention: parseMention,
	}
}
