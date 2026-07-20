const avoidParseRequire = require
const crypto = avoidParseRequire('node:crypto')

export default crypto
export const createCipheriv = crypto.createCipheriv
export const createHash = crypto.createHash
export const createHmac = crypto.createHmac
export const publicEncrypt = crypto.publicEncrypt
export const randomBytes = crypto.randomBytes
export const constants = crypto.constants
