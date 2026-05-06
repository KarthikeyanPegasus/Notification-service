import { useAuth, useClerk, useUser } from '@clerk/nextjs'
import { useMockAuth, useMockClerk, useMockUser } from './mock-clerk-provider'

function isClerkEnabled() {
  return (typeof process !== 'undefined' && process.env?.NEXT_PUBLIC_CLERK_ENABLED !== 'false')
}

export function useMaybeAuth() {
  // eslint-disable-next-line react-hooks/rules-of-hooks
  return isClerkEnabled() ? useAuth() : useMockAuth()
}

export function useMaybeUser() {
  // eslint-disable-next-line react-hooks/rules-of-hooks
  return isClerkEnabled() ? useUser() : useMockUser()
}

export function useMaybeClerk() {
  // eslint-disable-next-line react-hooks/rules-of-hooks
  return isClerkEnabled() ? useClerk() : useMockClerk()
}

