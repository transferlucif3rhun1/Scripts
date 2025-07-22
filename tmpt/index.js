const { extractTmptCookie } = require('./tmpt');

extractTmptCookie()
    .then(result => console.log(result))
    .catch(error => console.error(error));