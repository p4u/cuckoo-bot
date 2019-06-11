# Cuckoo telegram bot

This is a telegram bot to schedule tasks over the week.

These are the basic commands accepted by the bot.

+ /add <name> <weekDay> <hour> <message>
+ /list
+ /noisy <name> <periodMinutes>
+ /stop <name>

So, let's add a reminder for every day (weekday=0 means all days):

`/add dog 0 20:30 Give the dog food`

And another to call mum only on sunday:

`/add mum 7 21:00 Call mum`

Then, let's make the dog reminder noisy, so it will repeat every 10 minutes until we stop it:

`/noisy dog 10`

Once it start remaining, let's stop it:

`/stop dog`

To see the list of schedule tasks, we can use `/list`

I started the integration of an inline keyboard (to make it easier) but it's not finished yet.

## Disclaimer

I made this code just to understand Golang-Telegram integration and to supply an inmediate need in my house.
However I aknowladge that the code is not as good as I'd like to be... sorry for that but I'm not spending too much time with this :P

