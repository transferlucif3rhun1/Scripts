const express = require('express');
const bodyParser = require('body-parser');
const cluster = require('cluster');
const os = require('os');

const numCPUs = os.cpus().length;

if (cluster.isMaster) {
    console.log(`Master ${process.pid} is running`);

    // Fork workers
    for (let i = 0; i < numCPUs; i++) {
        cluster.fork();
    }

    cluster.on('exit', (worker, code, signal) => {
        console.log(`Worker ${worker.process.pid} died`);
        cluster.fork(); // Replace the dead worker
    });
} else {
    const app = express();
    const port = 3009;

    app.use(bodyParser.urlencoded({ extended: true }));

    // The single function where you'll paste your logic
    function encrypt(SKI, randomNum, Key, password) {
function Encrypt(newPassword) {
  var myArr = [];
  myArr = PackageNewPwdOnly(newPassword);
  var resultInB64 = RSAEncrypt(myArr, publicKey, randomNum);
  return resultInB64;
}
function PackageNewPwdOnly(newPassword) {
  var myArr = [];
  var pos = 0;
  myArr[pos++] = 1;
  myArr[pos++] = 1;
  var i;
  var passwordlen = newPassword.length;
  myArr[pos++] = passwordlen;
  for (i = 0; i < passwordlen; i++) {
    myArr[pos++] = newPassword.charCodeAt(i) & 0x7f;
  }
  myArr[pos++] = 0;
  myArr[pos++] = 0;
  return myArr;
}
function mapByteToBase64(ch) {
  if (ch >= 0 && ch < 26) {
    return String.fromCharCode(65 + ch);
  } else if (ch >= 26 && ch < 52) {
    return String.fromCharCode(97 + ch - 26);
  } else if (ch >= 52 && ch < 62) {
    return String.fromCharCode(48 + ch - 52);
  } else if (ch == 62) {
    return "+";
  }
  return "/";
}
function base64Encode(number, symcount) {
  var k,
    result = "";
  for (k = symcount; k < 4; k++) {
    number = number >> 6;
  }
  for (k = 0; k < symcount; k++) {
    result = mapByteToBase64(number & 0x3f) + result;
    number = number >> 6;
  }
  return result;
}
function byteArrayToBase64(arg) {
  var len = arg.length;
  var result = "";
  var i;
  var number;
  for (i = len - 3; i >= 0; i -= 3) {
    number = arg[i] | (arg[i + 1] << 8) | (arg[i + 2] << 16);
    result = result + base64Encode(number, 4);
  }
  var remainder = len % 3;
  number = 0;
  for (i += 2; i >= 0; i--) {
    number = (number << 8) | arg[i];
  }
  if (remainder == 1) {
    result = result + base64Encode(number << 16, 2) + "==";
  } else if (remainder == 2) {
    result = result + base64Encode(number << 8, 3) + "=";
  }
  return result;
}
function parseRSAKeyFromString(keytext) {
  var scIndex = keytext.indexOf(";");
  if (scIndex < 0) {
    return null;
  }
  var left = keytext.substr(0, scIndex);
  var right = keytext.substr(scIndex + 1);
  var eqIndex = left.indexOf("=");
  if (eqIndex < 0) {
    return null;
  }
  var exponentTxt = left.substr(eqIndex + 1);
  eqIndex = right.indexOf("=");
  if (eqIndex < 0) {
    return null;
  }
  var modulusTxt = right.substr(eqIndex + 1);
  var key = new Object();
  key["n"] = hexStringToMP(modulusTxt);
  key["e"] = parseInt(exponentTxt, 16);
  return key;
}
function RSAEncrypt(plaintext, publicKey, rndSource, randomNum) {
  var ciphertext = [];
  var minPaddingReq = 42;
  var blockSize = publicKey.n.size * 2 - minPaddingReq;
  for (var i = 0; i < plaintext.length; i += blockSize) {
    if (i + blockSize >= plaintext.length) {
      var encryptedBlock = RSAEncryptBlock(
        plaintext.slice(i),
        publicKey,
        randomNum
      );
      if (encryptedBlock) {
        ciphertext = encryptedBlock.concat(ciphertext);
      }
    } else {
      var encryptedBlock = RSAEncryptBlock(
        plaintext.slice(i, i + blockSize),
        publicKey,
        randomNum
      );
      if (encryptedBlock) {
        ciphertext = encryptedBlock.concat(ciphertext);
      }
    }
  }
  var resultInB64 = byteArrayToBase64(ciphertext);
  return resultInB64;
}
function RSAEncryptBlock(block, publicKey, rndSource) {
  var modulus = publicKey.n;
  var exponent = publicKey.e;
  var ptBytes = block.length;
  var modBytes = modulus.size * 2;
  var minPaddingReq = 42;
  if (ptBytes + minPaddingReq > modBytes) {
    return null;
  }
  applyPKCSv2Padding(block, modBytes, rndSource);
  block = block.reverse();
  var base = byteArrayToMP(block);
  var baseToExp = modularExp(base, exponent, modulus);
  baseToExp.size = modulus.size;
  var ciphertext = mpToByteArray(baseToExp);
  ciphertext = ciphertext.reverse();
  return ciphertext;
}
function JSMPnumber() {
  this.size = 1;
  this.data = [];
  this.data[0] = 0;
}
JSMPnumber.prototype = {
  size: 1,
  data: [0],
};
function duplicateMP(input) {
  var dup = new JSMPnumber();
  dup.size = input.size;
  dup.data = input.data.slice(0);
  return dup;
}
function byteArrayToMP(input) {
  var result = new JSMPnumber();
  var i = 0,
    bc = input.length;
  var half = bc >> 1;
  for (i = 0; i < half; i++) {
    result.data[i] = input[2 * i] + (input[1 + 2 * i] << 8);
  }
  if (bc % 2) {
    result.data[i++] = input[bc - 1];
  }
  result.size = i;
  return result;
}
function mpToByteArray(input) {
  var result = [];
  var i = 0,
    bc = input.size;
  for (i = 0; i < bc; i++) {
    result[i * 2] = input.data[i] & 0xff;
    var t = input.data[i] >>> 8;
    result[i * 2 + 1] = t;
  }
  return result;
}
function modularExp(base, power, modulus) {
  var bits = [];
  var bc = 0;
  while (power > 0) {
    bits[bc] = power & 1;
    power = power >>> 1;
    bc++;
  }
  var result = duplicateMP(base);
  for (var i = bc - 2; i >= 0; i--) {
    result = modularMultiply(result, result, modulus);
    if (bits[i] == 1) {
      result = modularMultiply(result, base, modulus);
    }
  }
  return result;
}
function modularMultiply(left, right, modulus) {
  var product = multiplyMP(left, right);
  var divResult = divideMP(product, modulus);
  return divResult.r;
}
function multiplyMP(left, right) {
  var product = new JSMPnumber();
  product.size = left.size + right.size;
  var i, j;
  for (i = 0; i < product.size; i++) {
    product.data[i] = 0;
  }
  var ld = left.data,
    rd = right.data,
    pd = product.data;
  if (left == right) {
    for (i = 0; i < left.size; i++) {
      pd[2 * i] += ld[i] * ld[i];
    }
    for (i = 1; i < left.size; i++) {
      for (j = 0; j < i; j++) {
        pd[i + j] += 2 * ld[i] * ld[j];
      }
    }
  } else {
    for (i = 0; i < left.size; i++) {
      for (j = 0; j < right.size; j++) {
        pd[i + j] += ld[i] * rd[j];
      }
    }
  }
  normalizeJSMP(product);
  return product;
}
function normalizeJSMP(number) {
  var i, carry, cb, word;
  var diff, original;
  cb = number.size;
  carry = 0;
  for (i = 0; i < cb; i++) {
    word = number.data[i];
    word += carry;
    original = word;
    carry = Math.floor(word / 0x10000);
    word -= carry * 0x10000;
    number.data[i] = word;
  }
}
function removeLeadingZeroes(number) {
  var i = number.size - 1;
  while (i > 0 && number.data[i--] == 0) {
    number.size--;
  }
}
function divideMP(number, divisor) {
  var nw = number.size;
  var dw = divisor.size;
  var msw = divisor.data[dw - 1];
  var mswDiv = divisor.data[dw - 1] + divisor.data[dw - 2] / 0x10000;
  var quotient = new JSMPnumber();
  quotient.size = nw - dw + 1;
  number.data[nw] = 0;
  for (var i = nw - 1; i >= dw - 1; i--) {
    var shift = i - dw + 1;
    var estimate = Math.floor(
      (number.data[i + 1] * 0x10000 + number.data[i]) / mswDiv
    );
    if (estimate > 0) {
      var sign = multiplyAndSubtract(number, estimate, divisor, shift);
      if (sign < 0) {
        estimate--;
        multiplyAndSubtract(number, estimate, divisor, shift);
      }
      while (sign > 0 && number.data[i] >= msw) {
        sign = multiplyAndSubtract(number, 1, divisor, shift);
        if (sign > 0) {
          estimate++;
        }
      }
    }
    quotient.data[shift] = estimate;
  }
  removeLeadingZeroes(number);
  var result = {
    q: quotient,
    r: number,
  };
  return result;
}
function multiplyAndSubtract(number, scalar, divisor, offset) {
  var i;
  var backup = number.data.slice(0);
  var carry = 0;
  var nd = number.data;
  for (i = 0; i < divisor.size; i++) {
    var temp = carry + divisor.data[i] * scalar;
    carry = temp >>> 16;
    temp = temp - carry * 0x10000;
    if (temp > nd[i + offset]) {
      nd[i + offset] += 0x10000 - temp;
      carry++;
    } else {
      nd[i + offset] -= temp;
    }
  }
  if (carry > 0) {
    nd[i + offset] -= carry;
  }
  if (nd[i + offset] < 0) {
    number.data = backup.slice(0);
    return -1;
  }
  return +1;
}
function applyPKCSv2Padding(message, modulusSize, rndsrc) {
  var mlen = message.length;
  var i;
  var lHash = [
    0xda, 0x39, 0xa3, 0xee, 0x5e, 0x6b, 0x4b, 0x0d, 0x32, 0x55, 0xbf, 0xef,
    0x95, 0x60, 0x18, 0x90, 0xaf, 0xd8, 0x07, 0x09,
  ];
  var padlen = modulusSize - mlen - 40 - 2;
  var PS = [];
  for (i = 0; i < padlen; i++) {
    PS[i] = 0x00;
  }
  PS[padlen] = 0x01;
  var DB = lHash.concat(PS, message);
  var seed = [];
  for (i = 0; i < 20; i++) {
    seed[i] = Math.floor(Math.random() * 256);
  }
  seed = SHA1(seed.concat(rndsrc));
  var dbMask = MGF(seed, modulusSize - 21);
  var maskedDB = XORarrays(DB, dbMask);
  var seedMask = MGF(maskedDB, 20);
  var maskedSeed = XORarrays(seed, seedMask);
  var encodedMsg = [];
  encodedMsg[0] = 0x00;
  encodedMsg = encodedMsg.concat(maskedSeed, maskedDB);
  for (i = 0; i < encodedMsg.length; i++) {
    message[i] = encodedMsg[i];
  }
}
function MGF(seed, masklen) {
  if (masklen > 0x1000) {
    return null;
  }
  var dup = seed.slice(0);
  var sl = dup.length;
  dup[sl++] = 0;
  dup[sl++] = 0;
  dup[sl++] = 0;
  dup[sl] = 0;
  var counter = 0;
  var T = [];
  while (T.length < masklen) {
    dup[sl] = counter++;
    T = T.concat(SHA1(dup));
  }
  return T.slice(0, masklen);
}
function XORarrays(left, right) {
  if (left.length != right.length) {
    return null;
  }
  var result = [];
  var end = left.length;
  for (var i = 0; i < end; i++) {
    result[i] = left[i] ^ right[i];
  }
  return result;
}
function SHA1(data) {
  var i;
  var dup = data.slice(0);
  PadSHA1Input(dup);
  var chainedState = {
    A: 0x67452301,
    B: 0xefcdab89,
    C: 0x98badcfe,
    D: 0x10325476,
    E: 0xc3d2e1f0,
  };
  for (i = 0; i < dup.length; i += 64) {
    SHA1RoundFunction(chainedState, dup, i);
  }
  var result = [];
  wordToBytes(chainedState.A, result, 0);
  wordToBytes(chainedState.B, result, 4);
  wordToBytes(chainedState.C, result, 8);
  wordToBytes(chainedState.D, result, 12);
  wordToBytes(chainedState.E, result, 16);
  return result;
}
function wordToBytes(number, dest, offset) {
  var i;
  for (i = 3; i >= 0; i--) {
    dest[offset + i] = number & 0xff;
    number = number >>> 8;
  }
}
function PadSHA1Input(bytes) {
  var unpadded = bytes.length;
  var len = unpadded;
  var inc = unpadded % 64;
  var completeTo = inc < 55 ? 56 : 120;
  var i;
  bytes[len++] = 0x80;
  for (i = inc + 1; i < completeTo; i++) {
    bytes[len++] = 0;
  }
  var unpBitCount = unpadded * 8;
  for (i = 1; i < 8; i++) {
    bytes[len + 8 - i] = unpBitCount & 0xff;
    unpBitCount = unpBitCount >>> 8;
  }
}
function SHA1RoundFunction(chainVar, block, offset) {
  var y1 = 0x5a827999,
    y2 = 0x6ed9eba1,
    y3 = 0x8f1bbcdc,
    y4 = 0xca62c1d6;
  var r, j, w;
  var words = [];
  var A = chainVar.A,
    B = chainVar.B,
    C = chainVar.C,
    D = chainVar.D,
    E = chainVar.E;
  for (j = 0, w = offset; j < 16; j++, w += 4) {
    words[j] =
      (block[w] << 24) |
      (block[w + 1] << 16) |
      (block[w + 2] << 8) |
      (block[w + 3] << 0);
  }
  for (j = 16; j < 80; j++) {
    words[j] = rotateLeft(
      words[j - 3] ^ words[j - 8] ^ words[j - 14] ^ words[j - 16],
      1
    );
  }
  var t;
  for (r = 0; r < 20; r++) {
    t =
      (rotateLeft(A, 5) + ((B & C) | (~B & D)) + E + words[r] + y1) &
      0xffffffff;
    E = D;
    D = C;
    C = rotateLeft(B, 30);
    B = A;
    A = t;
  }
  for (r = 20; r < 40; r++) {
    t = (rotateLeft(A, 5) + (B ^ C ^ D) + E + words[r] + y2) & 0xffffffff;
    E = D;
    D = C;
    C = rotateLeft(B, 30);
    B = A;
    A = t;
  }
  for (r = 40; r < 60; r++) {
    t =
      (rotateLeft(A, 5) + ((B & C) | (B & D) | (C & D)) + E + words[r] + y3) &
      0xffffffff;
    E = D;
    D = C;
    C = rotateLeft(B, 30);
    B = A;
    A = t;
  }
  for (r = 60; r < 80; r++) {
    t = (rotateLeft(A, 5) + (B ^ C ^ D) + E + words[r] + y4) & 0xffffffff;
    E = D;
    D = C;
    C = rotateLeft(B, 30);
    B = A;
    A = t;
  }
  chainVar.A = (chainVar.A + A) & 0xffffffff;
  chainVar.B = (chainVar.B + B) & 0xffffffff;
  chainVar.C = (chainVar.C + C) & 0xffffffff;
  chainVar.D = (chainVar.D + D) & 0xffffffff;
  chainVar.E = (chainVar.E + E) & 0xffffffff;
}
function rotateLeft(number, shift) {
  var lower = number >>> (32 - shift);
  var mask = (1 << (32 - shift)) - 1;
  var upper = number & mask;
  return (upper << shift) | lower;
}
function hexStringToMP(hexstr) {
  var i, hexWord;
  var cWords = Math.ceil(hexstr.length / 4);
  var result = new JSMPnumber();
  result.size = cWords;
  for (i = 0; i < cWords; i++) {
    hexWord = hexstr.substr(i * 4, 4);
    result.data[cWords - 1 - i] = parseInt(hexWord, 16);
  }
  return result;
}
randomNum = randomNum;
publicKey = parseRSAKeyFromString(Key);
cipher=Encrypt(password);
        return cipher;
    }

    // Endpoint to accept parameters and return the cipher
    app.post('/encrypt', (req, res) => {
        const {randomNum, Key, password } = req.body;

        try {
            const cipher = encrypt(SKI, randomNum, Key, password);
            res.json({ cipher });
        } catch (error) {
            res.status(500).json({ error: 'Internal Server Error' });
        }
    });

    app.listen(port, () => {
        console.log(`Worker ${process.pid} running at http://localhost:${port}`);
    });
}
