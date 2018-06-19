package objectserver

import (
	"go.uber.org/zap"
	context "golang.org/x/net/context"
)

type KVService struct {
	store *KVStore
}

func NewKVService(store *KVStore) *KVService {
	return &KVService{store}
}

func (k *KVService) SaveAsyncJob(
	ctx context.Context, msg *SaveAsyncJobMsg) (*SaveAsyncJobReply, error) {
	err := k.store.SaveAsyncJob(msg.Job)
	if err != nil {
		glogger.Error("unable to save async job", zap.Error(err))
	}

	return &SaveAsyncJobReply{Success: err == nil}, nil
}

func (k *KVService) ListAsyncJobs(
	ctx context.Context, msg *ListAsyncJobsMsg) (*ListAsyncJobsReply, error) {
	reply := &ListAsyncJobsReply{}
	var err error
	reply.Jobs, err = k.store.ListAsyncJobs(
		msg.Device, int(msg.Policy), msg.Position, int(msg.Pagination))
	if err != nil {
		glogger.Error("unable to list async jobs", zap.Error(err))
	}

	return reply, nil
}

func (k *KVService) CleanAsyncJob(
	ctx context.Context, msg *CleanAsyncJobMsg) (*CleanAsyncJobReply, error) {
	err := k.store.CleanAsyncJob(msg.Job)
	if err != nil {
		glogger.Error("unable to clean async job", zap.Error(err))
	}

	return &CleanAsyncJobReply{Success: err == nil}, nil
}