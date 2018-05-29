package main

import (
	pb "github.com/agxp/cloudflix/comments-svc/proto"
	"golang.org/x/net/context"
	"github.com/opentracing/opentracing-go"
	"go.uber.org/zap"
	"github.com/gogo/protobuf/proto"
)

type service struct {
	repo Repository
	tracer *opentracing.Tracer
	logger *zap.Logger
}

func (srv *service) GetAllForVideoId(ctx context.Context, req *pb.Request, res *pb.Response) error {
	sp, _ := opentracing.StartSpanFromContext(context.TODO(), "GetAllForVideoId_Service")

	logger.Info("Request for GetAllForVideoId_Service received")
	defer sp.Finish()

	comments, err := srv.repo.GetAllForVideoId(sp.Context(), req.VideoId)
	if err != nil {
		logger.Error("failed GetAllForVideoId", zap.Error(err))
		return err
	}

	res.Comments = comments.Comments

	sp.LogKV("comments",comments)

	return nil
}

func (srv *service) GetSingle(ctx context.Context, req *pb.SingleRequest, res *pb.Comment) error {
	sp, _ := opentracing.StartSpanFromContext(context.TODO(), "GetSingle_Service")
	logger.Info("Request for GetSingle_Service received")
	defer sp.Finish()

	comment, err := srv.repo.GetSingle(sp.Context(), req.CommentId)
	if err != nil {
		logger.Error("failed GetSingle", zap.Error(err))
		return err
	}

	data, err := proto.Marshal(comment)
	if err != nil {
		logger.Error("marshal error", zap.Error(err))
		return err
	}

	err = proto.Unmarshal(data, res)
	if err != nil {
		logger.Error("unmarshal error", zap.Error(err))
		return err
	}

	sp.LogKV("comment", comment)

	return nil
}


func (srv *service) Write(ctx context.Context, req *pb.WriteRequest, res *pb.Comment) error {
	sp, _ := opentracing.StartSpanFromContext(context.TODO(), "Write_Service")

	logger.Info("Request for Write_Service received")
	defer sp.Finish()

	comment, err := srv.repo.Write(sp.Context(), req)
	if err != nil {
		logger.Error("failed Write", zap.Error(err))
		return err
	}

	data, err := proto.Marshal(comment)
	if err != nil {
		logger.Error("marshal error", zap.Error(err))
		return err
	}

	err = proto.Unmarshal(data, res)
	if err != nil {
		logger.Error("unmarshal error", zap.Error(err))
		return err
	}

	sp.LogKV("comment", comment)

	return nil
}