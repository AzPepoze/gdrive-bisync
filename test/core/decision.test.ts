/**
 * To run this test file, you can use ts-node:
 * > npx ts-node test/core/decision.test.ts
 */

import * as assert from "assert";
import { determineSyncAction } from "../../src/core/decision";
import { DriveFile, FileMetadata, LocalFile, SyncAction } from "../../src/types";

console.log("Running tests for determineSyncAction...");

// --- Test Data ---
const baseTime = new Date().getTime();
const localFile: LocalFile = {
	path: "test.txt",
	mtime: new Date(baseTime).toISOString(),
	md5Checksum: "local_md5",
	isDirectory: false,
};
const remoteFile: DriveFile = {
	id: "remote_id",
	name: "test.txt",
	path: "test.txt",
	modifiedTime: new Date(baseTime).toISOString(),
	md5Checksum: "remote_md5",
	isDirectory: false,
};
const syncedMetadata: FileMetadata = { remoteMd5Checksum: "remote_md5_old" };

// --- Test Cases ---

// Test 1: New local file
assert.strictEqual(
	determineSyncAction("new_local.txt", { ...localFile, path: "new_local.txt" }, undefined, new Map()),
	SyncAction.UPLOAD_NEW,
	"Test 1 Failed: Should detect a new local file to upload."
);

// Test 2: New remote file
assert.strictEqual(
	determineSyncAction("new_remote.txt", undefined, { ...remoteFile, path: "new_remote.txt" }, new Map()),
	SyncAction.DOWNLOAD_NEW,
	"Test 2 Failed: Should detect a new remote file to download."
);

// Test 3: Local file deleted
assert.strictEqual(
	determineSyncAction(
		"deleted_local.txt",
		undefined,
		{ ...remoteFile, path: "deleted_local.txt" },
		new Map([["deleted_local.txt", syncedMetadata]])
	),
	SyncAction.DELETE_REMOTE,
	"Test 3 Failed: Should detect a local deletion."
);

// Test 4: Remote file deleted
assert.strictEqual(
	determineSyncAction(
		"deleted_remote.txt",
		{ ...localFile, path: "deleted_remote.txt" },
		undefined,
		new Map([["deleted_remote.txt", syncedMetadata]])
	),
	SyncAction.DELETE_LOCAL,
	"Test 4 Failed: Should detect a remote deletion."
);

// Test 5: Identical files (based on MD5)
assert.strictEqual(
	determineSyncAction(
		"identical.txt",
		{ ...localFile, path: "identical.txt", md5Checksum: "same_md5" },
		{ ...remoteFile, path: "identical.txt", md5Checksum: "same_md5" },
		new Map()
	),
	SyncAction.SKIP_IDENTICAL,
	"Test 5 Failed: Should skip identical files."
);

// Test 6: Local file updated (remote unchanged since last sync)
const metadataWithCurrentMd5 = new Map([["updated_local.txt", { remoteMd5Checksum: "current_remote_md5" }]]);
assert.strictEqual(
	determineSyncAction(
		"updated_local.txt",
		{ ...localFile, path: "updated_local.txt", md5Checksum: "new_local_md5" },
		{ ...remoteFile, path: "updated_local.txt", md5Checksum: "current_remote_md5" },
		metadataWithCurrentMd5
	),
	SyncAction.UPLOAD_UPDATE,
	"Test 6 Failed: Should upload the updated local file."
);

// Test 7: Remote file updated (newer timestamp)
const metadataForRemoteUpdate = new Map([["updated_remote.txt", { remoteMd5Checksum: "old_remote_md5" }]]);
assert.strictEqual(
	determineSyncAction(
		"updated_remote.txt",
		{ ...localFile, path: "updated_remote.txt", mtime: new Date(baseTime).toISOString() },
		{
			...remoteFile,
			path: "updated_remote.txt",
			md5Checksum: "new_remote_md5",
			modifiedTime: new Date(baseTime + 5000).toISOString(),
		},
		metadataForRemoteUpdate
	),
	SyncAction.DOWNLOAD_UPDATE,
	"Test 7 Failed: Should download the updated remote file."
);

// Test 8: Conflict (local newer or same time, but remote content changed)
const metadataForConflict = new Map([["conflict.txt", { remoteMd5Checksum: "old_remote_md5" }]]);
assert.strictEqual(
	determineSyncAction(
		"conflict.txt",
		{ ...localFile, path: "conflict.txt", mtime: new Date(baseTime + 10000).toISOString() },
		{
			...remoteFile,
			path: "conflict.txt",
			md5Checksum: "new_remote_md5", // Different from metadata
			modifiedTime: new Date(baseTime).toISOString(),
		},
		metadataForConflict
	),
	SyncAction.UPLOAD_CONFLICT,
	"Test 8 Failed: Should detect a conflict and favor local upload."
);

console.log("All tests passed!");
