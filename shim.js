'use strict';

const os = require('os');
const childProcess = require('child_process');

const archMap = {
	'x64': 'amd64',
	'arm64': 'arm64',
	'arm': 'arm',
	'ia32': '386'
};

const platform = os.platform();
const arch = os.arch();

const GOOS = platform === 'win32' ? 'windows' : platform;
const GOARCH = archMap[arch] || arch;

const binary = `${__dirname}/action-go-shim-${GOOS}-${GOARCH}${GOOS === 'windows' ? '.exe' : ''}`

const result = childProcess.spawnSync(binary, process.argv.slice(2), {
	stdio: 'inherit',
	shell: false
});

if (result.error) {
	console.error('shim: failed to spawn process:', result.error);
	process.exit(1);
}

process.exit(result.status !== null ? result.status : 1);
