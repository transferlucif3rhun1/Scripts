const exe = require('@angablue/exe');

const build = exe({
    entry: './reese84.js',
    out: './Incapsula solving redirection tool.exe',
    productVersion: '1.0.0',
    fileVersion: '1.0.0',
    target: 'latest-win-x64',
    properties: {
        FileDescription: 'Localhost forwarding service',
        ProductName: 'Incapsula solving redirection tool',
        LegalCopyright: 'Lucif3rHun1',
        OriginalFilename: 'Incapsula solving redirection tool.exe'
    }
});

build.then(() => console.log('Build completed!'));