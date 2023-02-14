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

package processing

import (
	"github.com/superseriousbusiness/gotosocial/internal/concurrency"
	"github.com/superseriousbusiness/gotosocial/internal/db"
	"github.com/superseriousbusiness/gotosocial/internal/email"
	"github.com/superseriousbusiness/gotosocial/internal/federation"
	"github.com/superseriousbusiness/gotosocial/internal/media"
	"github.com/superseriousbusiness/gotosocial/internal/messages"
	"github.com/superseriousbusiness/gotosocial/internal/oauth"
	"github.com/superseriousbusiness/gotosocial/internal/processing/account"
	"github.com/superseriousbusiness/gotosocial/internal/processing/admin"
	federationProcessor "github.com/superseriousbusiness/gotosocial/internal/processing/federation"
	mediaProcessor "github.com/superseriousbusiness/gotosocial/internal/processing/media"
	"github.com/superseriousbusiness/gotosocial/internal/processing/report"
	"github.com/superseriousbusiness/gotosocial/internal/processing/status"
	"github.com/superseriousbusiness/gotosocial/internal/processing/streaming"
	"github.com/superseriousbusiness/gotosocial/internal/processing/user"
	"github.com/superseriousbusiness/gotosocial/internal/storage"
	"github.com/superseriousbusiness/gotosocial/internal/timeline"
	"github.com/superseriousbusiness/gotosocial/internal/typeutils"
	"github.com/superseriousbusiness/gotosocial/internal/visibility"
)

type Processor struct {
	clientWorker *concurrency.WorkerPool[messages.FromClientAPI]
	fedWorker    *concurrency.WorkerPool[messages.FromFederator]

	federator       federation.Federator
	tc              typeutils.TypeConverter
	oauthServer     oauth.Server
	mediaManager    media.Manager
	storage         *storage.Driver
	statusTimelines timeline.Manager
	db              db.DB
	filter          visibility.Filter

	/*
		SUB-PROCESSORS
	*/

	account.AccountProcessor
	admin.AdminProcessor
	federationProcessor.FederationProcessor
	statusProcessor    status.Processor
	streamingProcessor streaming.Processor
	mediaProcessor     mediaProcessor.Processor
	userProcessor      user.Processor
	reportProcessor    report.Processor
}

// NewProcessor returns a new Processor.
func NewProcessor(
	tc typeutils.TypeConverter,
	federator federation.Federator,
	oauthServer oauth.Server,
	mediaManager media.Manager,
	storage *storage.Driver,
	db db.DB,
	emailSender email.Sender,
	clientWorker *concurrency.WorkerPool[messages.FromClientAPI],
	fedWorker *concurrency.WorkerPool[messages.FromFederator],
) *Processor {
	parseMentionFunc := GetParseMentionFunc(db, federator)

	statusProcessor := status.New(db, tc, clientWorker, parseMentionFunc)
	streamingProcessor := streaming.New(db, oauthServer)
	mediaProcessor := mediaProcessor.New(db, tc, mediaManager, federator.TransportController(), storage)
	userProcessor := user.New(db, emailSender)
	reportProcessor := report.New(db, tc, clientWorker)
	filter := visibility.NewFilter(db)

	return &Processor{
		clientWorker: clientWorker,
		fedWorker:    fedWorker,

		federator:       federator,
		tc:              tc,
		oauthServer:     oauthServer,
		mediaManager:    mediaManager,
		storage:         storage,
		statusTimelines: timeline.NewManager(StatusGrabFunction(db), StatusFilterFunction(db, filter), StatusPrepareFunction(db, tc), StatusSkipInsertFunction()),
		db:              db,
		filter:          visibility.NewFilter(db),

		AccountProcessor:    account.New(db, tc, mediaManager, oauthServer, clientWorker, federator, parseMentionFunc),
		AdminProcessor:      admin.New(db, tc, mediaManager, federator.TransportController(), storage, clientWorker),
		FederationProcessor: federationProcessor.New(db, tc, federator),

		statusProcessor:    statusProcessor,
		streamingProcessor: streamingProcessor,
		mediaProcessor:     mediaProcessor,
		userProcessor:      userProcessor,
		reportProcessor:    reportProcessor,
	}
}

// Start starts the Processor, reading from its channels and passing messages back and forth.
func (p *Processor) Start() error {
	// Setup and start the client API worker pool
	p.clientWorker.SetProcessor(p.ProcessFromClientAPI)
	if err := p.clientWorker.Start(); err != nil {
		return err
	}

	// Setup and start the federator worker pool
	p.fedWorker.SetProcessor(p.ProcessFromFederator)
	if err := p.fedWorker.Start(); err != nil {
		return err
	}

	// Start status timelines
	if err := p.statusTimelines.Start(); err != nil {
		return err
	}

	return nil
}

// Stop stops the processor cleanly, finishing handling any remaining messages before closing down.
func (p *Processor) Stop() error {
	if err := p.clientWorker.Stop(); err != nil {
		return err
	}

	if err := p.fedWorker.Stop(); err != nil {
		return err
	}

	if err := p.statusTimelines.Stop(); err != nil {
		return err
	}

	return nil
}
