// Copyright 2017 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"context"
	"fmt"
	"net/url"
	"path"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/storage"
	"code.gitea.io/gitea/modules/timeutil"
	"code.gitea.io/gitea/modules/util"
)

// Attachment represent a attachment of issue/comment/release.
type Attachment struct {
	ID            int64  `xorm:"pk autoincr"`
	UUID          string `xorm:"uuid UNIQUE"`
	RepoID        int64  `xorm:"INDEX"`           // this should not be zero
	IssueID       int64  `xorm:"INDEX"`           // maybe zero when creating
	ReleaseID     int64  `xorm:"INDEX"`           // maybe zero when creating
	UploaderID    int64  `xorm:"INDEX DEFAULT 0"` // Notice: will be zero before this column added
	CommentID     int64
	Name          string
	DownloadCount int64              `xorm:"DEFAULT 0"`
	Size          int64              `xorm:"DEFAULT 0"`
	CreatedUnix   timeutil.TimeStamp `xorm:"created"`
}

func init() {
	db.RegisterModel(new(Attachment))
}

// IncreaseDownloadCount is update download count + 1
func (a *Attachment) IncreaseDownloadCount() error {
	// Update download count.
	if _, err := db.GetEngine(db.DefaultContext).Exec("UPDATE `attachment` SET download_count=download_count+1 WHERE id=?", a.ID); err != nil {
		return fmt.Errorf("increase attachment count: %w", err)
	}

	return nil
}

// AttachmentRelativePath returns the relative path
func AttachmentRelativePath(uuid string) string {
	return path.Join(uuid[0:1], uuid[1:2], uuid)
}

// RelativePath returns the relative path of the attachment
func (a *Attachment) RelativePath() string {
	return AttachmentRelativePath(a.UUID)
}

// DownloadURL returns the download url of the attached file
func (a *Attachment) DownloadURL() string {
	return setting.AppURL + "attachments/" + url.PathEscape(a.UUID)
}

//    _____   __    __                .__                           __
//   /  _  \_/  |__/  |______    ____ |  |__   _____   ____   _____/  |_
//  /  /_\  \   __\   __\__  \ _/ ___\|  |  \ /     \_/ __ \ /    \   __\
// /    |    \  |  |  |  / __ \\  \___|   Y  \  Y Y  \  ___/|   |  \  |
// \____|__  /__|  |__| (____  /\___  >___|  /__|_|  /\___  >___|  /__|
//         \/                \/     \/     \/      \/     \/     \/

// ErrAttachmentNotExist represents a "AttachmentNotExist" kind of error.
type ErrAttachmentNotExist struct {
	ID   int64
	UUID string
}

// IsErrAttachmentNotExist checks if an error is a ErrAttachmentNotExist.
func IsErrAttachmentNotExist(err error) bool {
	_, ok := err.(ErrAttachmentNotExist)
	return ok
}

func (err ErrAttachmentNotExist) Error() string {
	return fmt.Sprintf("attachment does not exist [id: %d, uuid: %s]", err.ID, err.UUID)
}

func (err ErrAttachmentNotExist) Unwrap() error {
	return util.ErrNotExist
}

// GetAttachmentByID returns attachment by given id
func GetAttachmentByID(ctx context.Context, id int64) (*Attachment, error) {
	attach := &Attachment{}
	if has, err := db.GetEngine(ctx).ID(id).Get(attach); err != nil {
		return nil, err
	} else if !has {
		return nil, ErrAttachmentNotExist{ID: id, UUID: ""}
	}
	return attach, nil
}

// GetAttachmentByUUID returns attachment by given UUID.
func GetAttachmentByUUID(ctx context.Context, uuid string) (*Attachment, error) {
	attach := &Attachment{}
	has, err := db.GetEngine(ctx).Where("uuid=?", uuid).Get(attach)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrAttachmentNotExist{0, uuid}
	}
	return attach, nil
}

// GetAttachmentsByUUIDs returns attachment by given UUID list.
func GetAttachmentsByUUIDs(ctx context.Context, uuids []string) ([]*Attachment, error) {
	if len(uuids) == 0 {
		return []*Attachment{}, nil
	}

	// Silently drop invalid uuids.
	attachments := make([]*Attachment, 0, len(uuids))
	return attachments, db.GetEngine(ctx).In("uuid", uuids).Find(&attachments)
}

// ExistAttachmentsByUUID returns true if attachment exists with the given UUID
func ExistAttachmentsByUUID(ctx context.Context, uuid string) (bool, error) {
	return db.GetEngine(ctx).Where("`uuid`=?", uuid).Exist(new(Attachment))
}

// GetAttachmentsByIssueID returns all attachments of an issue.
func GetAttachmentsByIssueID(ctx context.Context, issueID int64) ([]*Attachment, error) {
	attachments := make([]*Attachment, 0, 10)
	return attachments, db.GetEngine(ctx).Where("issue_id = ? AND comment_id = 0", issueID).Find(&attachments)
}

// GetAttachmentsByCommentID returns all attachments if comment by given ID.
func GetAttachmentsByCommentID(ctx context.Context, commentID int64) ([]*Attachment, error) {
	attachments := make([]*Attachment, 0, 10)
	return attachments, db.GetEngine(ctx).Where("comment_id=?", commentID).Find(&attachments)
}

// GetAttachmentByReleaseIDFileName returns attachment by given releaseId and fileName.
func GetAttachmentByReleaseIDFileName(ctx context.Context, releaseID int64, fileName string) (*Attachment, error) {
	attach := &Attachment{ReleaseID: releaseID, Name: fileName}
	has, err := db.GetEngine(ctx).Get(attach)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, err
	}
	return attach, nil
}

// DeleteAttachment deletes the given attachment and optionally the associated file.
func DeleteAttachment(a *Attachment, remove bool) error {
	_, err := DeleteAttachments(db.DefaultContext, []*Attachment{a}, remove)
	return err
}

// DeleteAttachments deletes the given attachments and optionally the associated files.
func DeleteAttachments(ctx context.Context, attachments []*Attachment, remove bool) (int, error) {
	if len(attachments) == 0 {
		return 0, nil
	}

	ids := make([]int64, 0, len(attachments))
	for _, a := range attachments {
		ids = append(ids, a.ID)
	}

	cnt, err := db.GetEngine(ctx).In("id", ids).NoAutoCondition().Delete(attachments[0])
	if err != nil {
		return 0, err
	}

	if remove {
		for i, a := range attachments {
			if err := storage.Attachments.Delete(a.RelativePath()); err != nil {
				return i, err
			}
		}
	}
	return int(cnt), nil
}

// DeleteAttachmentsByIssue deletes all attachments associated with the given issue.
func DeleteAttachmentsByIssue(issueID int64, remove bool) (int, error) {
	attachments, err := GetAttachmentsByIssueID(db.DefaultContext, issueID)
	if err != nil {
		return 0, err
	}

	return DeleteAttachments(db.DefaultContext, attachments, remove)
}

// DeleteAttachmentsByComment deletes all attachments associated with the given comment.
func DeleteAttachmentsByComment(commentID int64, remove bool) (int, error) {
	attachments, err := GetAttachmentsByCommentID(db.DefaultContext, commentID)
	if err != nil {
		return 0, err
	}

	return DeleteAttachments(db.DefaultContext, attachments, remove)
}

// UpdateAttachmentByUUID Updates attachment via uuid
func UpdateAttachmentByUUID(ctx context.Context, attach *Attachment, cols ...string) error {
	if attach.UUID == "" {
		return fmt.Errorf("attachment uuid should be not blank")
	}
	_, err := db.GetEngine(ctx).Where("uuid=?", attach.UUID).Cols(cols...).Update(attach)
	return err
}

// UpdateAttachment updates the given attachment in database
func UpdateAttachment(ctx context.Context, atta *Attachment) error {
	sess := db.GetEngine(ctx).Cols("name", "issue_id", "release_id", "comment_id", "download_count")
	if atta.ID != 0 && atta.UUID == "" {
		sess = sess.ID(atta.ID)
	} else {
		// Use uuid only if id is not set and uuid is set
		sess = sess.Where("uuid = ?", atta.UUID)
	}
	_, err := sess.Update(atta)
	return err
}

// DeleteAttachmentsByRelease deletes all attachments associated with the given release.
func DeleteAttachmentsByRelease(releaseID int64) error {
	_, err := db.GetEngine(db.DefaultContext).Where("release_id = ?", releaseID).Delete(&Attachment{})
	return err
}

// CountOrphanedAttachments returns the number of bad attachments
func CountOrphanedAttachments() (int64, error) {
	return db.GetEngine(db.DefaultContext).Where("(issue_id > 0 and issue_id not in (select id from issue)) or (release_id > 0 and release_id not in (select id from `release`))").
		Count(new(Attachment))
}

// DeleteOrphanedAttachments delete all bad attachments
func DeleteOrphanedAttachments() error {
	_, err := db.GetEngine(db.DefaultContext).Where("(issue_id > 0 and issue_id not in (select id from issue)) or (release_id > 0 and release_id not in (select id from `release`))").
		Delete(new(Attachment))
	return err
}
