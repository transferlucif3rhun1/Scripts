// ----------------------
// Fingerprint Generator
// ----------------------

// Utility functions
const randomString = (length) => {
    const chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
    return Array.from({ length }, () => chars.charAt(Math.floor(Math.random() * chars.length))).join('');
  };
  
  const randomNum = (min, max) => Math.floor(Math.random() * (max - min + 1)) + min;
  
  const randomFloat = (min, max) => Math.random() * (max - min) + min;
  
  // Updated available fonts (matching sample payload)
  const availableFonts = [
    "Calibri",
    "Leelawadee",
    "MS UI Gothic",
    "Marlett",
    "Microsoft Uighur",
    "Segoe UI Light",
    "Small Fonts"
  ];
  
  // GPU models: NVIDIA only for now
  const gpuModels = [
    "NVIDIA GeForce RTX 5090",
    "NVIDIA GeForce RTX 5080",
    "NVIDIA GeForce RTX 4090",
    "NVIDIA GeForce RTX 4080",
    "NVIDIA GeForce RTX 3070 Ti",
    "NVIDIA GeForce GTX 1080",
    "NVIDIA GeForce GTX 980",
    "NVIDIA GeForce GTX 1660 Super",
    "NVIDIA Quadro P4000",
    "NVIDIA TITAN X",
    "NVIDIA GeForce MX450",
    "NVIDIA A100 Tensor Core GPU"
  ];
  
  // Available time zones (IANA zones)
  const availableTimezones = [
    "Africa/Abidjan", "Africa/Accra", "Africa/Cairo", "Africa/Johannesburg", "Africa/Nairobi",
    "America/Argentina/Buenos_Aires", "America/Chicago", "America/Guatemala", "America/Los_Angeles",
    "America/New_York", "America/Sao_Paulo", "America/Toronto", "Asia/Bangkok", "Asia/Dubai",
    "Asia/Hong_Kong", "Asia/Kathmandu", "Asia/Kolkata", "Asia/Seoul", "Asia/Shanghai",
    "Asia/Singapore", "Asia/Tehran", "Asia/Tokyo", "Australia/Adelaide", "Australia/Melbourne",
    "Australia/Perth", "Australia/Sydney", "Europe/Amsterdam", "Europe/Athens", "Europe/Berlin",
    "Europe/Istanbul", "Europe/London", "Europe/Madrid", "Europe/Moscow", "Europe/Paris",
    "Europe/Rome", "Europe/Warsaw", "Pacific/Auckland", "Pacific/Fiji", "Pacific/Honolulu"
  ];
  
  // Updated available plugins (matching sample payload)
  const availablePlugins = [
    { name: "PDF Viewer", description: "Portable Document Format", mimeTypes: [
        { type: "application/pdf", suffixes: "pdf" },
        { type: "text/pdf", suffixes: "pdf" }
      ]
    },
    { name: "Chrome PDF Viewer", description: "Portable Document Format", mimeTypes: [
        { type: "application/pdf", suffixes: "pdf" },
        { type: "text/pdf", suffixes: "pdf" }
      ]
    },
    { name: "Chromium PDF Viewer", description: "Portable Document Format", mimeTypes: [
        { type: "application/pdf", suffixes: "pdf" },
        { type: "text/pdf", suffixes: "pdf" }
      ]
    },
    { name: "Microsoft Edge PDF Viewer", description: "Portable Document Format", mimeTypes: [
        { type: "application/pdf", suffixes: "pdf" },
        { type: "text/pdf", suffixes: "pdf" }
      ]
    },
    { name: "WebKit built-in PDF", description: "Portable Document Format", mimeTypes: [
        { type: "application/pdf", suffixes: "pdf" },
        { type: "text/pdf", suffixes: "pdf" }
      ]
    }
  ];
  
  // Helper functions to pick random fonts, plugins, and a timezone
  const getRandomFonts = () => {
    const count = randomNum(5, 12);
    const fonts = [];
    for (let i = 0; i < count; i++) {
      fonts.push( availableFonts[randomNum(0, availableFonts.length - 1)] );
    }
    return fonts;
  };
  
  const getRandomPlugins = () => {
    const count = randomNum(4, availablePlugins.length);
    const plugins = [];
    for (let i = 0; i < count; i++) {
      plugins.push( availablePlugins[randomNum(0, availablePlugins.length - 1)] );
    }
    return plugins;
  };
  
  const getRandomTimezone = () => availableTimezones[randomNum(0, availableTimezones.length - 1)];
  
  // Fixed math fingerprint (as provided in your sample)
  const generateMathFingerprint = () => {
    return {
      acos: 1.4473588658278522,
      acosh: 709.889355822726,
      acoshPf: 355.291251501643,
      asin: 0.12343746096704435,
      asinh: 0.881373587019543,
      asinhPf: 0.8813735870195429,
      atanh: 0.5493061443340548,
      atanhPf: 0.5493061443340548,
      atan: 0.4636476090008061,
      sin: 0.8178819121159085,
      sinh: 1.1752011936438014,
      sinhPf: 2.534342107873324,
      cos: -0.8390715290095377,
      cosh: 1.5430806348152437,
      coshPf: 1.5430806348152437,
      tan: -1.4214488238747245,
      tanh: 0.7615941559557649,
      tanhPf: 0.7615941559557649,
      exp: 2.718281828459045,
      expm1: 1.718281828459045,
      expm1Pf: 1.718281828459045,
      log1p: 2.3978952727983707,
      log1pPf: 2.3978952727983707,
      powPI: 1.9275814160560206e-50
    };
  };
  
  // Main dynamic generation function
  const generateFingerprintData = () => {
    return {
      fonts: {
        value: getRandomFonts(),
        duration: randomNum(5, 100)
      },
      domBlockers: {
        duration: randomNum(5, 20)
      },
      fontPreferences: {
        value: {
          default: randomFloat(140, 150),
          apple: randomFloat(140, 150),
          serif: randomFloat(140, 150),
          sans: randomFloat(140, 150),
          mono: randomFloat(120, 130),
          min: randomFloat(8, 10),
          system: randomFloat(145, 150)
        },
        duration: randomNum(5, 30)
      },
      audio: {
        value: randomFloat(30, 40),
        duration: randomNum(5, 15)
      },
      screenFrame: {
        value: [0, 0, randomNum(40, 60), 0],
        duration: randomNum(0, 10)
      },
      osCpu: {
        value: `Windows NT ${randomNum(10, 11)}.0; Win64; x64`,
        duration: randomNum(0, 10)
      },
      languages: {
        value: [["en-GB"], ["en-GB", "en"]],
        duration: randomNum(0, 10)
      },
      colorDepth: {
        value: randomNum(24, 32),
        duration: randomNum(0, 10)
      },
      deviceMemory: {
        duration: randomNum(0, 10)
      },
      screenResolution: {
        value: [randomNum(1200, 1920), randomNum(800, 1080)],
        duration: randomNum(0, 10)
      },
      hardwareConcurrency: {
        value: randomNum(4, 16),
        duration: randomNum(0, 10)
      },
      timezone: {
        value: getRandomTimezone(),
        duration: randomNum(0, 10)
      },
      sessionStorage: {
        value: true,
        duration: randomNum(0, 10)
      },
      localStorage: {
        value: true,
        duration: randomNum(0, 10)
      },
      indexedDB: {
        value: true,
        duration: randomNum(0, 10)
      },
      openDatabase: {
        value: randomNum(0, 1) === 1,
        duration: randomNum(0, 10)
      },
      cpuClass: {
        duration: randomNum(0, 10)
      },
      platform: {
        value: "Win32",
        duration: randomNum(0, 10)
      },
      plugins: {
        value: getRandomPlugins(),
        duration: randomNum(0, 10)
      },
      canvas: {
        value: {
          winding: true,
          geometry: "data:image/png;base64,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",
          text: "data:image/png;base64,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"
        },
        duration: randomNum(5, 20)
      },
      touchSupport: {
        value: {
          maxTouchPoints: randomNum(0, 2),
          touchEvent: randomNum(0, 1) === 1,
          touchStart: randomNum(0, 1) === 1
        },
        duration: randomNum(0, 10)
      },
      vendor: {
        value: "",
        duration: randomNum(0, 10)
      },
      vendorFlavors: {
        value: [],
        duration: randomNum(0, 10)
      },
      colorGamut: {
        value: "srgb",
        duration: randomNum(0, 10)
      },
      invertedColors: {
        value: false,
        duration: randomNum(0, 10)
      },
      forcedColors: {
        value: false,
        duration: randomNum(0, 10)
      },
      monochrome: {
        value: 0,
        duration: randomNum(0, 10)
      },
      contrast: {
        value: 0,
        duration: randomNum(0, 10)
      },
      hdr: {
        value: false,
        duration: randomNum(0, 10)
      },
      videoCard: {
        value: {
          vendor: "Google Inc. (NVIDIA)",
          renderer: `ANGLE (NVIDIA, ${gpuModels[randomNum(0, gpuModels.length - 1)]} Direct3D11 vs_5_0 ps_5_0), or similar`
        },
        duration: randomNum(3, 10)
      },
      math: {
        value: generateMathFingerprint(),
        duration: randomNum(0, 5)
      },
      pdfViewerEnabled: {
        value: true,
        duration: randomNum(0, 5)
      },
      reducedMotion: {
        value: randomNum(0, 1) === 1,
        duration: randomNum(0, 5)
      },
      cookiesEnabled: {
        value: randomNum(0, 1) === 1,
        duration: randomNum(0, 5)
      },
      architecture: {
        value: 255, // Forced to 255 to match sample payload
        duration: randomNum(0, 5)
      }
    };
  };
  
  // ----------------------------------------
  // Express API to Return the fp_source JSON
  // ----------------------------------------
  const express = require('express');
  const app = express();
  const port = 3000;
  
  // When a GET request is made to /fp_source, return the fingerprint JSON
  app.get('/fp_source', (req, res) => {
    const fp_source = generateFingerprintData();
    res.json(fp_source);
  });
  
  // Start the server
  app.listen(port, () => {
    console.log(`Fingerprint API listening at http://localhost:${port}`);
  });
  