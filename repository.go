package main

import (
	"context"
	"database/sql"
	pb "github.com/agxp/cloudflix/comments-svc/proto"
	"github.com/minio/minio-go"
	"github.com/opentracing/opentracing-go"
	"go.uber.org/zap"
	"log"
	"github.com/go-redis/redis"
	"github.com/micro/protobuf/proto"
	"time"
	"crypto/md5"
	"encoding/hex"
)

type Repository interface {
	GetAllForVideoId(p opentracing.SpanContext, video_id string) (*pb.Response, error)
	GetSingle(p opentracing.SpanContext, id string) (*pb.Comment, error)
	Write(p opentracing.SpanContext, toWrite *pb.WriteRequest) (*pb.Comment, error)
}

type CommentRepository struct {
	s3     *minio.Client
	pg     *sql.DB
	cache  *redis.Client
	tracer *opentracing.Tracer
}

func (repo *CommentRepository) GetAllForVideoId(parent opentracing.SpanContext, video_id string) (*pb.Response, error) {
	sp, _ := opentracing.StartSpanFromContext(context.TODO(), "GetAllForVideoId_Repo", opentracing.ChildOf(parent))

	sp.LogKV("video_id", video_id)

	defer sp.Finish()

	// check for id in cache
	val, err := repo.cache.Get(video_id + "_comments").Result()
	if err == redis.Nil {
		// id not in cache
		psSP, _ := opentracing.StartSpanFromContext(context.TODO(), "PG_GetAllForVideoId", opentracing.ChildOf(sp.Context()))

		psSP.LogKV("video_id", video_id)

		selectQuery := `select id, user_id, date_created, content, likes, dislikes from comments where video_id=$1`
		rows, err := repo.pg.Query(selectQuery, video_id)
		if err != nil {
			log.Print(err)
			psSP.Finish()
			return nil, err
		}
		defer rows.Close()
		var comments []*pb.Comment
		for rows.Next() {
			var id string
			var user_id string
			var date_created string
			var content string
			var likes uint64
			var dislikes uint64

			if err := rows.Scan(&id, &user_id, &date_created, &content, &likes, &dislikes); err != nil {
				logger.Error(err.Error())
				psSP.Finish()
				return nil, err
			}

			c := &pb.Comment{
				Id:         id,
				VideoId:    video_id,
				User:       user_id,
				Content:    content,
				DatePosted: date_created,
				Likes:      likes,
				Dislikes:   dislikes,
			}

			comments = append(comments, c)
		}
		psSP.Finish()

		res := &pb.Response{
			Comments: comments,
		}

		v, err := proto.Marshal(res) // dumps the whole slice at once
		if err != nil {
			logger.Error("failed to marshal", zap.Error(err))
			return nil, err
		}

		// add res to cache
		repo.cache.Set(video_id+"_comments", v, 0)

		return res, nil

	} else if err != nil {
		logger.Error("cache error", zap.Error(err))
		return nil, err
	} else {
		// id found in cache
		logger.Info("cacheRes", zap.String("val", val))

		var res *pb.Response
		err = proto.Unmarshal([]byte(val), res)
		if err != nil {
			logger.Error("unmarshal error", zap.Error(err))
			return nil, err
		}

		return res, nil
	}

	return nil, nil
}

func (repo *CommentRepository) GetSingle(p opentracing.SpanContext, id string) (*pb.Comment, error) {
	sp, _ := opentracing.StartSpanFromContext(context.TODO(), "GetSingle_Repo", opentracing.ChildOf(p))

	sp.LogKV("id", id)

	defer sp.Finish()

	logger.Info("id", zap.String("id", id))
	var comment *pb.Comment
	// check for comment in cache
	c, err := repo.cache.Get("comment_" + id).Result()
	if err == redis.Nil {
		// comment not in cache
		// get comment from db
		dbSP, _ := opentracing.StartSpanFromContext(context.TODO(), "PG_GetSingle", opentracing.ChildOf(sp.Context()))

		dbSP.LogKV("id", id)

		var video_id string
		var user_id string
		var date_created string
		var content string
		var likes uint64
		var dislikes uint64

		selectQuery := `select video_id, user_id, date_created, content, likes, dislikes from comments where id=$1`
		err := repo.pg.QueryRow(selectQuery, id).Scan(&video_id, &user_id, &date_created, &content, &likes, &dislikes)
		if err != nil {
			logger.Error("couldn't get comment from db", zap.Error(err))
			dbSP.Finish()
			return nil, err
		}

		comment = &pb.Comment{
			Id:         id,
			VideoId:    video_id,
			User:       user_id,
			DatePosted: date_created,
			Content:    content,
			Likes:      likes,
			Dislikes:   dislikes,
		}

		v, err := proto.Marshal(comment)
		if err != nil {
			logger.Error("marshal error", zap.Error(err))
			return nil, err
		}

		repo.cache.Set("comment_"+id, v, 0)

		dbSP.Finish()

	} else if err != nil {
		logger.Error("cache error", zap.Error(err))
		return nil, err
	} else {
		// comment in cache
		// unmarshal

		err = proto.UnmarshalText(c, comment)
		if err != nil {
			logger.Error("unmarshal error", zap.Error(err))
			return nil, err
		}
	}

	return comment, nil

}

func (repo *CommentRepository) Write(parent opentracing.SpanContext, toWrite *pb.WriteRequest) (*pb.Comment, error) {
	sp, _ := opentracing.StartSpanFromContext(context.TODO(), "Write_Repo", opentracing.ChildOf(parent))

	sp.LogKV("toWrite", toWrite)

	defer sp.Finish()

	psSP, _ := opentracing.StartSpanFromContext(context.TODO(), "PG_Write", opentracing.ChildOf(sp.Context()))

	psSP.LogKV("toWrite", toWrite)

	insertQuery := `INSERT INTO comments(id, video_id, user_id, date_created, content, likes, dislikes) VALUES($1, $2, $3, $4, $5, $6, $7)`
	now := time.Now()
	c := now.String() + toWrite.VideoId + toWrite.Content
	hash := md5.Sum([]byte(c))
	commentId := hex.EncodeToString(hash[:])

	_, err := repo.pg.Exec(insertQuery, commentId, toWrite.VideoId, toWrite.User, now, toWrite.Content, 0, 0)
	if err != nil {
		log.Print(err)
		psSP.Finish()
		return nil, err
	}

	comment := &pb.Comment{
		Id:         commentId,
		VideoId:    toWrite.VideoId,
		User:       toWrite.User,
		Content:    toWrite.Content,
		DatePosted: now.String(),
		Likes:      0,
		Dislikes:   0,
	}

	psSP.Finish()

	v, err := proto.Marshal(comment) // dumps the whole slice at once
	if err != nil {
		logger.Error("failed to marshal", zap.Error(err))
		return nil, err
	}

	// add res to cache
	repo.cache.Set("comment_"+commentId, v, 0)

	return comment, nil

}
