const randomWidth = Math.round(Math.random() * (3840 - 1024) + 1024);
const randomHeight = Math.round(Math.random() * (2160 - 768) + 768);
const randomAvailWidth = Math.round(randomWidth * (0.8 + Math.random() * 0.2));
const randomAvailHeight = Math.round(
	randomHeight * (0.8 + Math.random() * 0.2)
);
const randomColorDepth = [1, 4, 8, 16, 24, 32][Math.floor(Math.random() * 6)];
const randomConcurrency = Math.floor(Math.random() * (16 - 2 + 1)) + 2;
const randomDevicePixelRatio = [1, 1.5, 2, 2.5, 3, 4][
	Math.floor(Math.random() * 6)
];
const subtypes = {
	application: ["json", "pdf", "xml", "zip"],
	audio: ["mp3", "ogg", "wav"],
	image: ["jpeg", "png", "gif", "svg+xml"],
	text: ["html", "css", "javascript", "plain"],
	video: ["mp4", "mpeg", "webm"],
};
const { Canvas } = require("skia-canvas");
const allMimeTypes = Object.entries(subtypes).flatMap(([type, subs]) =>
	subs.map((sub) => `${type}/${sub}`)
);
const randomMimeTypes = (count) =>
	allMimeTypes.sort(() => Math.random() - 0.5).slice(0, count);
const mimetypes = Iu(randomMimeTypes(5).sort().join(""));
function generateRandomPlugins(count) {
	const plugins = [
		"AdBlocker Pro",
		"Grammarly",
		"LastPass Password Manager",
		"Honey - Coupon Finder",
		"Dark Reader",
		"Loom Screen Recorder",
		"Google Translate",
		"React Developer Tools",
		"JSON Viewer",
		"Web Developer Toolbar",
		"uBlock Origin",
		"Stylus - Custom Themes",
		"ColorZilla",
		"Redux DevTools",
		"SEO Minion",
		"Page Ruler Redux",
		"VPN Proxy Extension",
		"Bitwarden Password Manager",
		"Zoom Scheduler",
		"Tab Suspender",
	];
	return plugins.sort(() => Math.random() - 0.5).slice(0, count);
}
const plugintypes = Iu(generateRandomPlugins(5).sort().join(""));
const randomizeWebGLRenderer = () => {
	const vendors = {
		Intel: ["HD Graphics", "Iris Graphics", "UHD Graphics"],
		NVIDIA: ["GeForce GTX 1050", "RTX 3060", "MX150"],
		AMD: ["Radeon RX Vega", "RX 560", "HD 7750"],
	};
	const vendor = Object.keys(vendors)[(Math.random() * 3) | 0];
	const model = vendors[vendor][(Math.random() * 3) | 0];
	return `ANGLE (${vendor}, ${vendor}(${model}) Direct3D11 vs_5_0 ps_5_0), or similar`;
};
const generateCanvasL = () => {
	const canvas = new Canvas(500, 100);
	const ctx = canvas.getContext("2d");
	const text = n(47) + String.fromCharCode(55357, 56836);
	(ctx.textBaseline = "alphabetic"), (ctx.fillStyle = "#f60");
	ctx.fillRect(125, 1, 62, 20);
	ctx.fillStyle = "#069";
	ctx.font = "13pt bogus-font-xxx";
	ctx.fillText(text, 2, 20);
	ctx.fillStyle = "rgba(102, 204, 0, 0.6123456789)";
	ctx.font = "16pt Arial";
	ctx.fillText(text, 4, 22);
	return canvas.toDataURLSync();
};
const generateCanvasY = () => {
	const canvas = new Canvas(400, 200);
	const ctx = canvas.getContext("2d");
	var t = 2 * Math.PI;
	ctx.globalCompositeOperation = "multiply";
	for (
		var u = 0,
			e = [
				["#f0f", 50, 50],
				["#0ff", 100, 50],
				["#f70", 75, 100],
			];
		u < e.length;
		u++
	) {
		var i = (f = e[u])[0],
			c = f[1],
			f = f[2];
		(ctx.fillStyle = i),
			ctx.beginPath(),
			ctx.arc(c, f, 50, 0, t, !0),
			ctx.closePath(),
			ctx.fill();
	}
	(ctx.fillStyle = "#70f"),
		ctx.arc(75, 75, 75, 0, t, !0),
		ctx.arc(75, 75, 25, 0, t, !0),
		ctx.fill("evenodd");
	return canvas.toDataURLSync();
};
const generateLYH = () => {
	const L = Iu(generateCanvasL()).toString(16);
	const Y = Iu(generateCanvasY()).toString(16);
	const H = randomizeWebGLRenderer();
	return {
		L,
		Y,
		H,
	};
};
function generateRandomArray(length) {
	const array = new Array(length);
	for (let i = 0; i < length; i++) {
		array[i] = Math.random() < 0.5 ? 0 : 1;
	}
	return array;
}
function generateRandomErrorDetails() {
	return {
		K: Math.floor(30000 + Math.random() * 10000),
		$: "too much recursion",
		U: "InternalError",
		W: Math.floor(7000 + Math.random() * 2000),
	};
}
var r = [
	"Onpxfcnpr",
	"Qryrgr",
	"Fcnpr",
	"Ragre",
	"Gno",
	"nhqvb",
	"ivqrb",
	"nqqRiragYvfgrare",
	"nggnpuRirag",
	"qrgnpuRirag",
	"erzbirRiragYvfgrare",
	"ba",
	"trgBjaCebcreglAnzrf",
	"trgCebgbglcrBs",
	"trgBjaCebcreglQrfpevcgbe",
	"cyngsbez",
	"iraqbe",
	"hfreNtrag",
	"cebqhpgFho",
	"ynathntr",
	"uneqjnerPbapheerapl",
	"qrivprZrzbel",
	"zvzrGlcrf",
	"wninRanoyrq",
	"pbbxvrRanoyrq",
	"perqragvnyf",
	"oyhrgbbgu",
	"fgbentr",
	"crezvffvbaf",
	"freivprJbexre",
	"qbAbgGenpx",
	"cyhtvaf",
	"ohvyqVQ",
	"qrcgu",
	"riny",
	"bfpch",
	"qbphzragZbqr",
	"puebzr",
	"bcren",
	"bce",
	"xrlhc,xrlqbja,pyvpx,zbhfrhc,zbhfrqbja,gbhpuraq,gbhpufgneg,gbhpupnapry,zbhfrzbir,gbhpuzbir,qrivprzbgvba,jurry",
	"gbhpurf",
	"ebgngvbaEngr",
	"nppryrebzrgre",
	"tlebfpbcr",
	"zntargbzrgre",
	"tenagrq",
	"Lkfxnsgohq, tr i\u00e5e JP-mbaz\u00f6 VD-uw\u00e4yc. ",
	"pnainf",
	"13cg obthf-sbag-kkk",
	"16cg Nevny",
	"eton(102, 204, 0, 0.6123456789)",
	"rirabqq",
	"2q",
	"jroty",
	"rkcrevzragny-",
	"JROTY_qroht_eraqrere_vasb",
	"flfgrzKQCV",
	"ybtvpnyKQCV",
	"qrivprCvkryEngvb",
	"unfNggevohgr",
	"nggevohgrf",
	"pnyyCunagbz",
	"_cunagbz",
	"cunagbz",
	"__cunagbznf",
	"pnyyFryravhz",
	"qevire",
	"rinyhngr",
	"fpevcg_sa",
	"fpevcg_shap",
	"fryravhz",
	"Fryravhz_VQR_Erpbeqre",
	"hajenccrq",
	"jroqevire",
	"avtugzner",
	"Frdhraghz",
	"qbzNhgbzngvba",
	"znkGbhpuCbvagf",
	"GbhpuRirag",
	"gbhpufgneg",
	"vachg",
	"rahzrengrQrivprf",
	"zrqvnQrivprf",
	"Sversbk",
	"Naqebvq",
	"trgGvzrmbarBssfrg",
	"cebonoyl",
	"znlor",
	"pnaCynlGlcr",
	"btt; pbqrpf='ibeovf'",
	"zcrt;",
	"jni; pbqrpf='1'",
	"k-z4n;",
	"nnp;",
	"btt; pbqrpf='gurben'",
	"zc4; pbqrpf='nip1.42R01R'",
	"jroz; pbqrpf='ic8, ibeovf'",
	"Znkvzhz pnyy fgnpx fvmr rkprrqrq",
	"gbb zhpu erphefvba",
	"Pnaabg ernq cebcregl 'o' bs haqrsvarq",
	"(ibvq 0) vf haqrsvarq",
	"haqrsvarq vf abg na bowrpg (rinyhngvat '(ibvq 0).o')",
	"Pnaabg ernq cebcregvrf bs haqrsvarq (ernqvat 'o')",
	"Reebe",
	"ZnpVagry",
	"Jva32",
	"vCubar",
	"Yvahk nezi8y",
	"ra-HF",
	"rf-RF",
	"se-SE",
	"cg-OE",
	"ra-TO",
	"qr-QR",
	"eh-EH",
	"Tbbtyr Vap.",
	"Nccyr Pbzchgre, Vap.",
	"Zngu",
];
function jn(n, r) {
	return Math.abs(n - r);
}
function n(n) {
	return (function (n) {
		for (var r, t, u, e = "", i = 13 % 26, c = 0; c < n.length; c++)
			e +=
				((t = i),
				65 <= (u = (r = n.charAt(c)).charCodeAt()) && u <= 90
					? String.fromCharCode(((u - 65 + t) % 26) + 65)
					: 97 <= u && u <= 122
					? String.fromCharCode(((u - 97 + t) % 26) + 97)
					: r);
		return e;
	})(r[n]);
}
function Ln(n, r) {
	return n && n.substring(0, r);
}
function On(n) {
	return ("0" + (255 & n).toString(16)).slice(-2);
}
function Jn(n) {
	for (var r = "", t = 0, u = n; t < u.length; t++) {
		var e = u[t];
		r += On(e);
	}
	return r;
}
function Zn(n, r) {
	for (
		var t, u, e = 2 * r, i = "", c = n.toString().split(""), f = [];
		c.length;

	)
		for (u = parseInt(c.shift(), 10), t = 0; u || t < f.length; t++)
			(u += 10 * (f[t] || 0)), (f[t] = u % 16), (u = (u - f[t]) / 16);
	for (; f.length; ) i += f.pop().toString(16);
	for (; i.length < e; ) i = "0" + i;
	return i;
}
function Bn(n, r) {
	for (
		var t = 0,
			u = r && n.length > r ? n.slice(0, r) : n,
			e = u.length,
			i = e - 1;
		0 <= i;
		i--
	)
		t |= (u[i] ? 1 : 0) << (e - i - 1);
	return r && e < r && (t <<= r - e), t;
}
function Hi() {
	var r = generateRandomArray(12),
		t = Bn(r, "");
	return On(r.length) + Zn(t, Math.ceil(r.length / 8));
}
function On(n) {
	return ("0" + (255 & n).toString(16)).slice(-2);
}
function Iu(n, r) {
	for (
		var t,
			u = 3 & n.length,
			e = n.length - u,
			i = r,
			c = 3432918353,
			f = 461845907,
			o = 0,
			a = 0;
		o < e;

	)
		(a =
			(255 & n.charCodeAt(o)) |
			((255 & n.charCodeAt(++o)) << 8) |
			((255 & n.charCodeAt(++o)) << 16) |
			((255 & n.charCodeAt(++o)) << 24)),
			++o,
			(i =
				(65535 &
					(t =
						(5 *
							(65535 &
								(i =
									((i ^= a =
										((65535 &
											(a =
												((a =
													((65535 & a) * c +
														((((a >>> 16) * c) & 65535) << 16)) &
													4294967295) <<
													15) |
												(a >>> 17))) *
											f +
											((((a >>> 16) * f) & 65535) << 16)) &
										4294967295) <<
										13) |
									(i >>> 19))) +
							(((5 * (i >>> 16)) & 65535) << 16)) &
						4294967295)) +
				27492 +
				((((t >>> 16) + 58964) & 65535) << 16));
	switch (((a = 0), u)) {
		case 3:
			a ^= (255 & n.charCodeAt(o + 2)) << 16;
		case 2:
			a ^= (255 & n.charCodeAt(o + 1)) << 8;
		case 1:
			i ^= a =
				((65535 &
					(a =
						((a =
							((65535 & (a ^= 255 & n.charCodeAt(o))) * c +
								((((a >>> 16) * c) & 65535) << 16)) &
							4294967295) <<
							15) |
						(a >>> 17))) *
					f +
					((((a >>> 16) * f) & 65535) << 16)) &
				4294967295;
	}
	return (
		(i ^= n.length),
		(i =
			(2246822507 * (65535 & (i ^= i >>> 16)) +
				(((2246822507 * (i >>> 16)) & 65535) << 16)) &
			4294967295),
		((i =
			(3266489909 * (65535 & (i ^= i >>> 13)) +
				(((3266489909 * (i >>> 16)) & 65535) << 16)) &
			4294967295) ^
			(i >>> 16)) >>>
			0
	);
}
function Zn(n, r) {
	for (
		var t, u, e = 2 * r, i = "", c = n.toString().split(""), f = [];
		c.length;

	)
		for (u = parseInt(c.shift(), 10), t = 0; u || t < f.length; t++)
			(u += 10 * (f[t] || 0)), (f[t] = u % 16), (u = (u - f[t]) / 16);
	for (; f.length; ) i += f.pop().toString(16);
	for (; i.length < e; ) i = "0" + i;
	return i;
}
function Yi(n, r) {
	return (n &= 32767) == (r &= 65535) ? Zn(32768 | n, 2) : Zn(n, 2) + Zn(r, 2);
}
function Tn(n) {
	return ("0" + (3 & n).toString(2)).slice(-2);
}
function qn(n, r) {
	for (var t = [], u = 0, e = n; u < e.length; u++) {
		var i = e[u];
		t.push(r(i));
	}
	return t;
}
function ui(n, r, t) {
	var u = On(((31 & n) << 3) | (7 & r));
	switch (r) {
		case 6:
			u += On(Math.round(10 * t));
			break;
		case 3:
			u += On(t);
			break;
		case 5:
			u += t <= 127 ? On(t) : Zn((1 << 15) | (32767 & t), 2);
			break;
		case 4:
			var e = (function (n, r) {
				for (
					var t, u, e = 0, i = [], c = 0;
					c < n.length &&
					((t = []),
					(u = n.charCodeAt(c)) < 128
						? t.push(u)
						: u < 2048
						? t.push(192 | (u >> 6), 128 | (63 & u))
						: u < 55296 || 57344 <= u
						? t.push(224 | (u >> 12), 128 | ((u >> 6) & 63), 128 | (63 & u))
						: ((u = 65536 + (((1023 & u) << 10) | (1023 & n.charCodeAt(++c)))),
						  t.push(
								240 | (u >> 18),
								128 | ((u >> 12) & 63),
								128 | ((u >> 6) & 63),
								128 | (63 & u)
						  )),
					!r || e + t.length <= r);
					c++
				)
					(e += t.length), i.push.apply(i, t);
				return i;
			})(Ln(t, 200), 255);
			u += On(e.length) + Jn(e);
			break;
		case 2:
		case 1:
			break;
		case 7:
			u += t;
	}
	return u;
}
function generateArray() {
	const LYH = generateLYH();
	const KUW = generateRandomErrorDetails();
	const randomArray = [];

	// Define each ui call output explicitly
	randomArray[0] = ui(0, 3, 1);
	randomArray[1] = ui(1, 4, "");
	randomArray[2] = ui(2, 3, 0);
	randomArray[3] = ui(
		4,
		7,
		Yi(randomWidth, randomAvailWidth) + Yi(randomHeight, randomAvailHeight)
	);
	randomArray[4] = ui(5, 5, randomColorDepth);
	randomArray[5] = ui(6, 5, randomConcurrency);
	randomArray[6] = ui(7, 6, randomDevicePixelRatio);
	randomArray[7] = ui(
		8,
		7,
		(() => {
			const t = new Date().getTimezoneOffset() / 15;
			const u = (() => {
				const n = new Date();
				n.setDate(1);
				n.setMonth(0);
				const r = new Date().getTimezoneOffset();
				n.setMonth(6);
				return jn(r, new Date().getTimezoneOffset()) / 15;
			})();
			return On(t) + On(u);
		})()
	);
	randomArray[8] = ui(9, 7, On(5) + Zn(mimetypes, 4));
	randomArray[9] = ui(10, 7, On(5) + Zn(plugintypes, 4));
	randomArray[10] = ui(11, 7, Hi());
	randomArray[11] = ui(
		12,
		4,
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:132.0) Gecko/20100101 Firefox/132.0"
	);
	randomArray[12] = ui(13, 4, LYH.L);
	randomArray[13] = ui(14, 7, "0307");
	randomArray[14] = ui(15, 2);
	randomArray[15] = ui(17, 3, 1);
	randomArray[16] = ui(18, 4, LYH.Y);
	randomArray[17] = ui(19, 4, LYH.H);
	randomArray[18] = ui(
		20,
		4,
		(() => {
			const c = new Date();
			c.setTime(0);
			return c.toLocaleString();
		})()
	);
	randomArray[19] = ui(21, 7, "0800");
	randomArray[20] = ui(22, 5, 37);
	randomArray[21] = ui(23, 3, 0);
	randomArray[22] = ui(24, 5, KUW.K);
	randomArray[23] = ui(25, 3, 2);
	randomArray[24] = ui(26, 3, 0);
	randomArray[25] = ui(27, 5, KUW.W);
	randomArray[26] = ui(28, 7, "00");
	randomArray[27] = ui(29, 3, 1);
	randomArray[28] = ui(30, 7, "38db3529b8");
	randomArray[29] = ui(
		31,
		7,
		Zn(parseInt(qn(generateRandomArray(8), Tn).join(""), 2), 2)
	);

	return randomArray;
}

var gu = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=";
function On(n) {
	return ("0" + (255 & n).toString(16)).slice(-2);
}
function Sn(n) {
	return (Math.random() * n) | 0;
}
Fn = function () {
	return new Date();
};
An =
	Date.now ||
	function () {
		return Fn().getTime();
	};
function Dn(n) {
	return (15 & n).toString(16);
}
function Gn(n, r) {
	for (var t = [], u = 0, e = 0, i = n.split(""); e < i.length; e++) {
		var c = i[e],
			c = parseInt(c, 16) ^ parseInt(r.charAt(u), 16);
		t.push(c.toString(16)), (u = (u + 1) % r.length);
	}
	return t.join("");
}
function Jn(n) {
	for (var r = "", t = 0, u = n; t < u.length; t++) {
		var e = u[t];
		r += On(e);
	}
	return r;
}
function bu(n, r, t, u) {
	var e = n.slice(0, r);
	return (
		(r = parseInt(t, 16)),
		(e = (n = e.split("")).length),
		Array.prototype.unshift.apply(n, Array.prototype.splice.call(n, r % e, e)),
		(t = n.join("")),
		Gn(u, t)
	);
}
function pc(e, i, c) {
	var r,
		t,
		u,
		f = generateArray(),
		o = f.length,
		a = [
			"c000",
			"0000000000000000000000000000000000000000000000000000000000000000404040404040000000000000000000000000400000",
			"000000000000000000",
		],
		v = On(Sn(256)),
		_ =
			((_ = An()),
			(_ = Math.floor(_ / 1000 - 1535000000)),
			(_ = Math.max(Math.min(_, 268435455), 0)),
			(u = Jn([_ >> 24, _ >> 16, _ >> 8, _])),
			(_ = Dn(Sn(16))),
			(u = Gn(u.substr(1, u.length), _)) + _),
		a = On(((7 & 0) << 5) | (31 & o)) + f.join("") + a.join("") + "ff",
		a = bu(_, 4, _.charAt(3), a),
		a = bu(e, 8, e.charAt(9), _ + a);
	return (function (r) {
		for (
			var t, u, e = String(r), i = 0, c = gu, f = "";
			e.charAt(0 | i) || ((c = "="), i % 1);
			f += c.charAt(63 & (t >> (8 - (i % 1) * 8)))
		) {
			if (255 < (u = e.charCodeAt((i += 3 / 4)))) return nn;
			t = (t << 8) | u;
		}
		return f;
	})(
		(v = (function () {
			for (
				var n = "", r = 0, t = (v + Gn(i + c + "84" + e + a, v)).match(/.{2}/g);
				r < t.length;
				r++
			) {
				var u = t[r];
				n += String.fromCharCode(255 & parseInt(u, 16));
			}
			return n;
		})())
	)
		.replace(/\+/g, "-")
		.replace(/\//g, "_")
		.replace(/=+$/, "");
}
function Zn(n, r) {
	for (
		var t, u, e = 2 * r, i = "", c = n.toString().split(""), f = [];
		c.length;

	)
		for (u = parseInt(c.shift(), 10), t = 0; u || t < f.length; t++)
			(u += 10 * (f[t] || 0)), (f[t] = u % 16), (u = (u - f[t]) / 16);
	for (; f.length; ) i += f.pop().toString(16);
	for (; i.length < e; ) i = "0" + i;
	return i;
}
var __cuid = require("crypto").randomBytes(16).toString("hex");
var castle_request_token = pc(__cuid, "05", Zn("391268837373533", 7));
console.log(__cuid);
console.log(castle_request_token);
