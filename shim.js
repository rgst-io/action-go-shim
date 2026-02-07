'use strict';

const os = require('os');
const childProcess = require('child_process');
const process = require('process');

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

// GITHUB_ACTION_PATH isn't set when uses: ./ is used, so in that case
// just default to the root of the repository.
if (!process.env.GITHUB_ACTION_PATH) {
	process.env.GITHUB_ACTION_PATH = __dirname
	console.warn(
		`[action-go-shim (js)] GITHUB_ACTION_PATH unset,`,
		`defaulting to shim dir (${process.env.GITHUB_ACTION_PATH})`,
	)
}

console.log(`[action-go-shim (js)] executing ${binary}`)
const result = childProcess.spawnSync(binary, process.argv.slice(2), {
	env: process.env,
	stdio: 'inherit',
	shell: false
});

if (result.error) {
	console.error('[action-go-shim (js)] failed to spawn process:', result.error);
	process.exit(1);
}

process.exit(result.status !== null ? result.status : 1);
