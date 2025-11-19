//-------------------------------------------------------
// From driveApi.ts
//-------------------------------------------------------
export interface DriveFile {
	id: string;
	name: string;
	path: string;
	modifiedTime: string;
	md5Checksum?: string;
	isDirectory: boolean;
}

export type DriveFileMap = Map<string, DriveFile>;

//-------------------------------------------------------
// From localScanner.ts
//-------------------------------------------------------
export interface LocalFile {
	path: string;
	mtime: string;
	md5Checksum?: string;
	isDirectory: boolean;
}

export type LocalFileMap = Map<string, LocalFile>;

//-------------------------------------------------------
// From mainSync.ts
//-------------------------------------------------------
export interface FileMetadata {
	remoteMd5Checksum?: string;
}

// Enum to make sync decisions clearer
export enum SyncAction {
	DOWNLOAD_NEW,
	DOWNLOAD_UPDATE,
	UPLOAD_NEW,
	UPLOAD_UPDATE,
	UPLOAD_CONFLICT,
	DELETE_LOCAL,
	DELETE_REMOTE,
	SKIP_IDENTICAL,
	SKIP_NO_CHANGE,
}

// A task to be executed
export interface SyncTask {
	action: SyncAction;
	filePath: string;
}
