package drive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type Client struct {
	svc *drive.Service
}

func New(ctx context.Context, credentialsFile string) (*Client, error) {
	svc, err := drive.NewService(ctx, option.WithCredentialsFile(credentialsFile))
	if err != nil {
		return nil, fmt.Errorf("drive.NewService: %w", err)
	}
	return &Client{svc: svc}, nil
}

// ListByPrefix returns files in folderID whose names start with prefix.
func (c *Client) ListByPrefix(ctx context.Context, folderID, prefix string) ([]*drive.File, error) {
	query := fmt.Sprintf("'%s' in parents and name contains '%s' and trashed=false", folderID, prefix)
	resp, err := c.svc.Files.List().
		Context(ctx).
		Q(query).
		Fields("files(id,name,createdTime)").
		Do()
	if err != nil {
		return nil, fmt.Errorf("files.list: %w", err)
	}
	return resp.Files, nil
}

// DownloadJSON fetches a Drive file and unmarshals it into dst.
func (c *Client) DownloadJSON(ctx context.Context, fileID string, dst any) error {
	resp, err := c.svc.Files.Get(fileID).Context(ctx).Download()
	if err != nil {
		return fmt.Errorf("files.get %s: %w", fileID, err)
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(dst)
}

// UploadJSON marshals src and uploads it to folderID with the given name.
// Returns the created file ID.
func (c *Client) UploadJSON(ctx context.Context, folderID, name string, src any) (string, error) {
	b, err := json.Marshal(src)
	if err != nil {
		return "", err
	}
	f := &drive.File{
		Name:    name,
		Parents: []string{folderID},
	}
	created, err := c.svc.Files.Create(f).
		Context(ctx).
		Media(strings.NewReader(string(b))).
		Fields("id").
		Do()
	if err != nil {
		return "", fmt.Errorf("files.create %s: %w", name, err)
	}
	return created.Id, nil
}

// UpdateJSON overwrites an existing Drive file's content.
func (c *Client) UpdateJSON(ctx context.Context, fileID string, src any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	_, err = c.svc.Files.Update(fileID, &drive.File{}).
		Context(ctx).
		Media(strings.NewReader(string(b))).
		Do()
	return err
}

// UploadFile uploads a local file to folderID. Returns the Drive file ID.
func (c *Client) UploadFile(ctx context.Context, folderID, localPath, mimeType string) (string, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return "", err
	}

	meta := &drive.File{
		Name:    stat.Name(),
		Parents: []string{folderID},
	}
	created, err := c.svc.Files.Create(meta).
		Context(ctx).
		Media(f).
		Fields("id,name,webViewLink").
		Do()
	if err != nil {
		return "", fmt.Errorf("files.create (upload) %s: %w", localPath, err)
	}

	// Make publicly readable so client can stream without service account auth
	_, err = c.svc.Permissions.Create(created.Id, &drive.Permission{
		Type: "anyone",
		Role: "reader",
	}).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("permissions.create: %w", err)
	}

	return created.Id, nil
}

// Delete removes a file from Drive.
func (c *Client) Delete(ctx context.Context, fileID string) error {
	return c.svc.Files.Delete(fileID).Context(ctx).Do()
}

// FileExists checks whether fileID exists in Drive.
func (c *Client) FileExists(ctx context.Context, fileID string) bool {
	_, err := c.svc.Files.Get(fileID).Context(ctx).Fields("id").Do()
	return err == nil
}

// DownloadStream returns a ReadCloser for the file content (caller must close).
func (c *Client) DownloadStream(ctx context.Context, fileID string) (io.ReadCloser, int64, error) {
	meta, err := c.svc.Files.Get(fileID).Context(ctx).Fields("size").Do()
	if err != nil {
		return nil, 0, fmt.Errorf("files.get meta %s: %w", fileID, err)
	}
	resp, err := c.svc.Files.Get(fileID).Context(ctx).Download()
	if err != nil {
		return nil, 0, fmt.Errorf("files.get download %s: %w", fileID, err)
	}
	return resp.Body, meta.Size, nil
}
