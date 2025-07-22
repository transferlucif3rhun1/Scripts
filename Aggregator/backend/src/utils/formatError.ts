module.exports = (formattedError) => {
  // You can tailor this to return specific error codes, etc.
  return {
    message: formattedError.message,
    code: formattedError.extensions?.code,
  };
};
