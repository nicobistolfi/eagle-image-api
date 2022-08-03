function toBoolean(value: any): boolean {
  return value === true || value === 'true' || value === 1 || value === '1'
}

// eslint-disable-next-line import/prefer-default-export
export { toBoolean }
