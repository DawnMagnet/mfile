module Main where

data Sample = Sample Int String

greet :: Sample -> String
greet (Sample i n) = show i ++ ":" ++ n

main :: IO ()
main = putStrLn (greet (Sample 1 "qa"))
